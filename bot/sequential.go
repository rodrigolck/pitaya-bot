package bot

import (
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/topfreegames/pitaya-bot/metrics"
	"github.com/topfreegames/pitaya-bot/models"
)

// SequentialBot defines the struct for the sequential bot that is going to run
type SequentialBot struct {
	client          *PClient
	config          *viper.Viper
	id              int
	spec            *models.Spec
	storage         *storage
	logger          logrus.FieldLogger
	host            string
	metricsReporter []metrics.Reporter
}

// NewSequentialBot returns a new sequantial bot instance
func NewSequentialBot(config *viper.Viper, spec *models.Spec, id int, mr []metrics.Reporter, logger logrus.FieldLogger) (Bot, error) {
	bot := &SequentialBot{
		config:          config,
		spec:            spec,
		id:              id,
		storage:         newStorage(config),
		logger:          logger,
		host:            config.GetString("server.host"),
		metricsReporter: mr,
	}

	if err := bot.Connect(); err != nil {
		return nil, err
	}

	return bot, nil
}

// Initialize initializes the bot
func (b *SequentialBot) Initialize() error {
	// TODO
	return nil
}

// Run runs the bot
func (b *SequentialBot) Run() error {
	defer b.Disconnect()

	steps := b.spec.SequentialOperations

	for _, step := range steps {
		err := b.runOperation(step)
		if err != nil {
			return err
		}
	}

	return nil
}

func (b *SequentialBot) runRequest(op *models.Operation) error {
	b.logger.Debug("Executing request to: " + op.URI)
	route := op.URI
	args, err := buildArgs(op.Args, b.storage)
	if err != nil {
		return err
	}

	resp, rawResp, err := sendRequest(args, route, b.client, b.metricsReporter)
	if err != nil {
		return err
	}

	b.logger.Debug("validating expectations")
	err = validateExpectations(op.Expect, resp, b.storage)
	if err != nil {
		return NewExpectError(err, rawResp, op.Expect)
	}
	b.logger.Debug("received valid response")

	b.logger.Debug("storing data")
	err = storeData(op.Store, b.storage, resp)
	if err != nil {
		return err
	}

	b.logger.Debug("all done")
	return nil
}

func (b *SequentialBot) runNotify(op *models.Operation) error {
	b.logger.Debug("Executing notify to: " + op.URI)
	route := op.URI
	args, err := buildArgs(op.Args, b.storage)
	if err != nil {
		return err
	}

	err = sendNotify(args, route, b.client)
	if err != nil {
		return err
	}

	b.logger.Debug("all done")
	return nil
}

func (b *SequentialBot) runFunction(op *models.Operation) error {
	fName := op.URI
	b.logger.Debug("Will execute internal function: ", fName)

	switch fName {
	case "disconnect":
		b.Disconnect()
	case "connect":
		host := b.host
		args, err := buildArgs(op.Args, b.storage)
		if err != nil {
			return err
		}
		if val, ok := args["host"]; ok {
			b.logger.Debug("Connecting to custom host")
			if h, ok := val.(string); ok {
				host = h
			}
		}
		b.Connect(host)
	case "reconnect":
		b.Reconnect()
	default:
		return fmt.Errorf("Unknown function: %s", fName)
	}

	return nil
}

func (b *SequentialBot) listenToPush(op *models.Operation) error {
	b.logger.Debug("Waiting for push on route: " + op.URI)
	resp, err := b.client.ReceivePush(op.URI, op.Timeout)
	if err != nil {
		return err
	}

	b.logger.Debug("validating expectations")
	err = validateExpectations(op.Expect, resp, b.storage)
	if err != nil {
		return err
	}
	b.logger.Debug("received valid response")

	b.logger.Debug("storing data")
	err = storeData(op.Store, b.storage, resp)
	if err != nil {
		return err
	}

	b.logger.Debug("all done")
	return nil
}

// StartListening ...
func (b *SequentialBot) startListening() {
	b.client.StartListening()
}

// TODO - refactor
func (b *SequentialBot) runOperation(op *models.Operation) error {
	switch op.Type {
	case "request":
		return b.runRequest(op)
	case "notify":
		return b.runNotify(op)
	case "function":
		return b.runFunction(op)
	case "listen":
		return b.listenToPush(op)
	}

	return fmt.Errorf("Unknown type: %s", op.Type)
}

// Finalize finalizes the bot
func (b *SequentialBot) Finalize() error {
	// TODO
	return nil
}

// Disconnect ...
func (b *SequentialBot) Disconnect() {
	b.client.Disconnect()
}

// Connect ...
func (b *SequentialBot) Connect(hosts ...string) error {
	if len(hosts) > 0 {
		b.host = hosts[0]
	}
	if b.client != nil && b.client.Connected() {
		b.logger.Fatal("Bot already connected")
	}

	client, err := NewPClient(b.host, b.config.GetBool("server.tls"))
	if err != nil {
		b.logger.Error("Unable to create client...")
		return err
	}

	b.client = client
	b.startListening()
	return nil
}

// Reconnect ...
func (b *SequentialBot) Reconnect() {
	b.Disconnect()
	b.Connect()
	b.logger.Debug("Reconnect done")
}
