package main

import (
	"errors"
	"fmt"
	"log"
	"log/syslog"
	"os"
	"os/exec"
	"time"

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

func connectAndServe(config *Config, cmdChannel chan string) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("Recovered", r)
		}
	}()

	amq := camq.GetAMQChannel(config.AMQ)
	amq.DeclareExchange()

	msgs, err := amq.GetExchangeChannel()

	if err != nil {
		log.Print(err)
		return
	}

	for d := range msgs {
		log.Printf("Received a message: %s\n", d.Body)
		cmdChannel <- string(d.Body)
	}
}

func getCommand(config Config, c string) (Command, error) {
	for _, cmd := range config.Commands {
		if cmd.ID == c {
			return cmd, nil
		}
	}
	return Command{}, errors.New("Not Found")
}

func main() {
	config := ReadConfig()
	cmdChannel := make(chan string)

	logwriter, err := syslog.New(syslog.LOG_NOTICE, "lightserver")
	if err != nil {
		log.SetOutput(logwriter)
	} else {
		log.Print("Unable to connect to syslog", err)
	}

	go func() {
		for {
			connectAndServe(&config, cmdChannel)
			time.Sleep(10 * time.Second)
			log.Print("Reconnecting")
		}
	}()

	for {
		cmd, err := getCommand(config, <-cmdChannel)
		if err != nil {
			log.Print(err)
			continue
		}
		go func(cmd Command) {
			c := exec.Command("bash", "-c", cmd.Cmd)
			c.Start()
			err = c.Wait()
			log.Printf("Command %s:%s finished with error: %v", cmd.ID, cmd.Cmd, err)
		}(cmd)
	}
}
