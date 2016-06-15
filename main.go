package main

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"log/syslog"
	"os"
	"os/exec"

	"github.com/BurntSushi/toml"
	"github.com/streadway/amqp"
)

type AMQConfig struct {
	URL        string
	Queue      string
	ClientKey  string
	CACert     string
	ServerCert string
}

type Config struct {
	AMQ      AMQConfig
	Commands []Command
}

type Command struct {
	ID  string
	Cmd string
}

type Amq struct {
	conn    *amqp.Connection
	channel *amqp.Channel
}

var amq Amq

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
	if err == nil {
		log.SetOutput(logwriter)
	} else {
		log.Print("Unable to connect to syslog", err)
	}

	cfg := new(tls.Config)

	cfg.RootCAs = x509.NewCertPool()
	cfg.RootCAs.AppendCertsFromPEM([]byte(config.AMQ.CACert))

	cert, _ := tls.X509KeyPair([]byte(config.AMQ.ServerCert), []byte(config.AMQ.ClientKey))
	cfg.Certificates = append(cfg.Certificates, cert)

	cfg.BuildNameToCertificate()
	conn, err := amqp.DialTLS(config.AMQ.URL, cfg)
	amq.conn = conn
	failOnError(err, "Failed to connect to RabbitMQ")
	defer amq.conn.Close()

	ch, err := amq.conn.Channel()
	failOnError(err, "Failed to create channel")
	amq.channel = ch
	defer amq.channel.Close()

	err = ch.ExchangeDeclare(config.AMQ.Queue, "fanout", false, false, false, false, nil)
	failOnError(err, "Failed to declare exchange")

	q, err := amq.channel.QueueDeclare("", false, false, true, false, nil)
	failOnError(err, "Failed to declare a queue")
	err = ch.QueueBind(q.Name, "", config.AMQ.Queue, false, nil)
	failOnError(err, "Failed to bind a queue")

	msgs, err := amq.channel.Consume(q.Name, "", true, false, false, false, nil)
	failOnError(err, "Failed to register a consumer")

	forever := make(chan bool)
	go func() {
		for d := range msgs {
			log.Printf("Received a message: %s\n", d.Body)
			bdy := string(d.Body)
			for _, cmd := range config.Commands {
				if cmd.ID == bdy {
					go func() {
						c := exec.Command("bash", "-c", cmd.Cmd)
						c.Start()
						err = c.Wait()
						log.Printf("Command finished with error: %s %v", cmd.Cmd, err)
					}()
				}
			}
		}
	}()
	<-forever
}
