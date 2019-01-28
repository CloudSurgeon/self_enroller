package main

import (
	"fmt"
	"os"
	"time"

	"github.com/jessevdk/go-flags"
	log "github.com/sirupsen/logrus"
)

// Options for the script
type Options struct {
	DDPName            string               `short:"e" long:"ddp_hostname" env:"DELPHIX_DDP_HOSTNAME" description:"The hostname or IP address of the Delphix Dynamic Data Platform" required:"true"`
	UserName           string               `short:"u" long:"username" env:"DELPHIX_USER" description:"The username used to authenticate to the Delphix Engine" required:"true"`
	Password           string               `short:"p" long:"password" env:"DELPHIX_PASS" description:"The password used to authenticate to the Delphix Engine" required:"true"`
	Debug              []bool               `short:"v" long:"debug" env:"DELPHIX_DEBUG" description:"Turn on debugging. -vvv for the most verbose debugging"`
	SkipValidate       bool                 `long:"skip-validate" env:"DELPHIX_SKIP_VALIDATE" description:"Don't validate TLS certificate of Delphix Engine"`
	ConfigFile         func(s string) error `short:"c" long:"config" description:"Optional INI config file to pass in for the variables" no-ini:"true"`
	EnvironmentName    string               `long:"environment" env:"DELPHIX_ENV" description:"optional: Specify the name of the environment in Delphix. Defaults to hostname"`
	KeyFile            flags.Filename       `long:"filename" description:"optional: The file to append the Delphix DDP public key. Creates file if it doesn't exist"`
	ToolKitPath        string               `long:"toolkit_path" env:"TOOLKIT_PATH" description:"The path for the toolkit that resides on the host" required:"true"`
	EnvironmentUser    string               `long:"environment_user" env:"ENVIRONMENT_USER" description:"The OS username to use for the environment" required:"true"`
	EnvironmentAddress string               `long:"environment_address" env:"ENVIRONMENT_ADDRESS" description:"optional: The address associated with the host."`
	RetryBadPass       bool                 `long:"retry-badpass" env:"RETRY_BAD_PASS" description:"Retry connection on bad password response (useful for waiting on new engine config)"`
}

func (c *Client) getSSHPublicKey() (key string, err error) {
	systemObj, err := c.httpGet("/system")
	if err != nil {
		return key, err
	}

	key, ok := systemObj["result"].(map[string]interface{})["sshPublicKey"].(string) //grab the object reference
	if !ok {
		err = fmt.Errorf("Did not find the sshPublicKey. Something went terribly wrong")
		return key, err
	}
	return key, err

}

func writeFile(filename, filetext string) (err error) {
	var f *os.File

	f, err = os.OpenFile(filename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}

	defer f.Close()

	if _, err = f.WriteString(filetext); err != nil {
		return err
	}
	return err
}

func (c *Client) writeDelphixPublicKey(filename string) (err error) {
	key, err := c.getSSHPublicKey()
	if err != nil {
		return err
	}
	err = writeFile(filename, key)
	if err != nil {
		return err
	}
	log.Infof("Delphix DDP key written to %s", filename)
	return err
}

func createHostEnvironmentCreateParameters(userName, environmentName, hostAddress, toolkitPath string) string {
	return fmt.Sprintf(`{
    "type": "HostEnvironmentCreateParameters",
    "primaryUser": {
        "type": "EnvironmentUser",
        "name": "%s",
        "credential": {
            "type": "SystemKeyCredential"
        }
    },
    "hostEnvironment": {
        "type": "UnixHostEnvironment",
        "name": "%s"
    },
    "hostParameters": {
        "type": "UnixHostCreateParameters",
        "host": {
            "type": "UnixHost",
            "address": "%s",
            "toolkitPath": "%s"
        }
    }
}`, userName, environmentName, hostAddress, toolkitPath)
}

func (c *Client) addEnvironment(userName, environmentName, hostAddress, toolkitPath string) (results map[string]interface{}, err error) {
	if envObj, err := c.findObjectByName("environment", environmentName); envObj == nil && err == nil {
		log.Debugf("%s", createHostEnvironmentCreateParameters(userName, environmentName, hostAddress, toolkitPath))
		action, err := c.httpPost("environment", createHostEnvironmentCreateParameters(userName, environmentName, hostAddress, toolkitPath))
		if err != nil {
			switch err.(type) {
			case *RespError:
				if err.(*RespError).ErrorID() == "exception.executor.object.exists" {
					log.Warn("Possibly encountered https://jira.delphix.com/browse/DLPX-46621, trying again in 5 seconds")
					time.Sleep(time.Duration(5) * time.Second)
					action, err = c.httpPost("environment", createHostEnvironmentCreateParameters(userName, environmentName, hostAddress, toolkitPath))
					if err == nil {
						break
					}
				}
				log.Fatal(err)
			default:
				log.Fatal("What the h* just happened?")
			}
		}
		err = c.jobWaiter(action)
		if err != nil {
			return nil, err
		}
		return action, err
	} else if err != nil {
		return nil, err
	} else {
		log.Warnf("%s already exists", environmentName)
	}
	return nil, err
}

var (
	opts             Options
	parser           = flags.NewParser(&opts, flags.Default)
	apiVersionString = "1.9.3"
	logger           *log.Entry
	url              string
	version          = "not set"
)

func main() {
	var err error
	var hostname, address string

	log.Info("Establishing session and logging in")
	client := NewClient(opts.UserName, opts.Password, fmt.Sprintf("https://%s/resources/json/delphix", opts.DDPName))
	client.initResty()

	// err = client.waitForEngineReady(10, 600)
	err = client.LoadAndValidate()
	if err != nil {
		log.Fatal(err)
	}

	log.Info("Successfully Logged in")
	err = client.writeDelphixPublicKey(string(opts.KeyFile))
	if err != nil {
		log.Fatal(err)
	}

	if hostname = opts.EnvironmentName; hostname == "" {
		hostname, err = os.Hostname()
		if err != nil {
			log.Fatal(err)
		}
	}
	if address = opts.EnvironmentAddress; address == "" {
		address = hostname
	}
	_, err = client.addEnvironment(opts.EnvironmentUser, address, hostname, opts.ToolKitPath)
	if err != nil {
		log.Fatal(err)
	}

	log.Info("Complete")
}
