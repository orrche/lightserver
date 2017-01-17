package main

import (
	"fmt"
	"log"
	"log/syslog"
	"os"
	"os/exec"

	"minoris.se/rabbitmq/camq"

	"github.com/BurntSushi/toml"
)

type Config struct {
	AMQ      camq.AMQConfig
	Commands []Command
}

type Command struct {
	ID  string
	Cmd string
}

func failOnError(err error, msg string) {
	if err != nil {
		log.Fatalf("%s: %s", msg, err)
		panic(fmt.Sprintf("%s: %s", msg, err))
	}
}

func ReadConfig() Config {
	var configfile = "conf/conf.toml"
	_, err := os.Stat(configfile)
	failOnError(err, "Config file missing")

	var config Config
	_, err = toml.DecodeFile(configfile, &config)
	failOnError(err, "")

	return config
}
func main() {
	config := ReadConfig()
	logwriter, err := syslog.New(syslog.LOG_NOTICE, "lightserver")
	if err != nil {
		log.SetOutput(logwriter)
	} else {
		log.Print("Unable to connect to syslog", err)
	}

	amq := camq.GetAMQChannel(config.AMQ)
	amq.DeclareExchange()

	msgs, err := amq.GetExchangeChannel()

	if err != nil {
		log.Print(err)
		return
	}

	for d := range msgs {
		log.Printf("Received a message: %s\n", d.Body)
		bdy := string(d.Body)
		for _, cmd := range config.Commands {
			if cmd.ID == bdy {
				go func(cmd Command) {
					c := exec.Command("bash", "-c", cmd.Cmd)
					c.Start()
					err = c.Wait()
					log.Printf("Command %s:%s finished with error: %v", cmd.ID, cmd.Cmd, err)
				}(cmd)
			}
		}
	}
}
