package options

import (
	"errors"
	"flag"
	"os"

	"github.com/icyrogue/ye-keeper/internal/api"
	"github.com/icyrogue/ye-keeper/internal/asyncstorageinterface"
	"github.com/icyrogue/ye-keeper/internal/client"
	"github.com/icyrogue/ye-keeper/internal/dbstorage"
	"github.com/icyrogue/ye-keeper/internal/notificationmanager"
	"github.com/icyrogue/ye-keeper/internal/queuemanager"
	"github.com/icyrogue/ye-keeper/internal/schemamanager"
	"github.com/icyrogue/ye-keeper/internal/usermanager"
)

type Config struct {
	DBOpts               *dbstorage.Options
	SchemaManagerOpts    *schemamanager.Options
	APIOpts              *api.Options
	QueueOpts            *queuemanager.Options
	StorageInterfaceOpts *asyncstorageinterface.Options
	ClientOpts           *client.Options
	UserManagerOpts      *usermanager.Options
	MailingOpts          *notificationmanager.Options
}

func Get() (*Config, error) {
	cfg := Config{
		DBOpts:               &dbstorage.Options{},
		SchemaManagerOpts:    &schemamanager.Options{},
		APIOpts:              &api.Options{},
		QueueOpts:            &queuemanager.Options{},
		StorageInterfaceOpts: &asyncstorageinterface.Options{},
		ClientOpts:           &client.Options{},
		UserManagerOpts:      &usermanager.Options{},
		MailingOpts:          &notificationmanager.Options{},
	}
	flag.StringVar(&cfg.DBOpts.Dsn, "d", "", "database dsn")
	if err := flag.Lookup("d").Value.Set(os.Getenv("KEEPER_DSN")); err != nil {
		return &cfg, err
	}
	flag.StringVar(&cfg.SchemaManagerOpts.Filepath, "f", "schemas.json", "path to lists schemas storage")
	flag.StringVar(&cfg.APIOpts.Port, "p", "8080", "port for api")
	flag.StringVar(&cfg.QueueOpts.Prefix, "q", "queueCache", "a place to store all cache from queue")
	flag.IntVar(&cfg.StorageInterfaceOpts.MaxWaitTime, "w", 30, "max wait time")
	flag.IntVar(&cfg.StorageInterfaceOpts.MaxBufferLength, "b", 30, "max buffer length for storage interface")
	flag.StringVar(&cfg.ClientOpts.MailTempPath, "ht", "", "path mail template")
	flag.IntVar(&cfg.ClientOpts.MaxTimeOutTime, "cwt", 60, "max wait time for client")
	flag.IntVar(&cfg.ClientOpts.MaxRequestsPer, "cmr", 10, "max req per cycle for client")

	flag.StringVar(&cfg.MailingOpts.KeeperMail, "addr", "", "mail address for mailing?")
	flag.StringVar(&cfg.MailingOpts.KeeperMailPasswd, "pswd", "", "password for mail address for mailing?")
	flag.StringVar(&cfg.MailingOpts.MailHost, "host", "", "host for mailing")

	var tmp string
	if tmp = os.Getenv("KEEPER_SECRET_KEY"); tmp == "" {
		return &cfg, errors.New("Mandatory value of KEEPER_SECRET_KEY isnt set")
	}
	cfg.UserManagerOpts.SecretKey = tmp

	if tmp = os.Getenv("KEEPER_MAIL_TEMPLATE_PATH"); tmp == "" {
		cfg.ClientOpts.MailTempPath = "misc/mail.html"
	}

	if tmp = os.Getenv("EFIND_API_TOKEN"); tmp == "" {
		return &cfg, errors.New("Mandatory value of EFIND_API_TOKEN isnt set")
	}
	cfg.ClientOpts.APIToken = tmp

	flag.Parse()
	return &cfg, nil
}
