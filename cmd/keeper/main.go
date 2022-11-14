package main

import (
	"context"
	"log"

	"github.com/icyrogue/ye-keeper/internal/api"
	"github.com/icyrogue/ye-keeper/internal/asyncstorageinterface"
	cachemanager "github.com/icyrogue/ye-keeper/internal/cacheManager"
	"github.com/icyrogue/ye-keeper/internal/client"
	"github.com/icyrogue/ye-keeper/internal/componentanalyzer"
	"github.com/icyrogue/ye-keeper/internal/dbstorage"
	"github.com/icyrogue/ye-keeper/internal/multiencoder"
	"github.com/icyrogue/ye-keeper/internal/notificationmanager"
	"github.com/icyrogue/ye-keeper/internal/options"
	"github.com/icyrogue/ye-keeper/internal/queuemanager"
	"github.com/icyrogue/ye-keeper/internal/requestprocessor"
	"github.com/icyrogue/ye-keeper/internal/schemamanager"
	"github.com/icyrogue/ye-keeper/internal/usermanager"
)

func main() {
	cfg, err := options.Get()
	if err != nil {
		log.Println(err.Error())
	}
	storage := dbstorage.New()

	storage.Options = cfg.DBOpts
	err = storage.Init()
	if err != nil {
		log.Println(err.Error())
	}
	defer storage.Close()
	userManager := usermanager.New(storage.GetPool())

	notificationManager := notificationmanager.New(userManager, cfg.MailingOpts)
	userManager.NotificationManager = notificationManager

	userManager.Options = cfg.UserManagerOpts
	err = userManager.Init()
	if err != nil {
		log.Println(err.Error())
	}

	schemaManager := schemamanager.New(storage)
	schemaManager.Options = cfg.SchemaManagerOpts
	err = schemaManager.Init()
	if err != nil {
		log.Println(err.Error())
	}

	output := make(chan []interface{}, 10)
	ctx := context.Background()

	multiEncoder := multiencoder.New(schemaManager)
	multiEncoder.StorageInterfaceInput = output

	analyzer := componentanalyzer.New(multiEncoder, schemaManager)
	multiEncoder.Output = analyzer.GetInput()

	analyzer.Start(ctx, output)

	storageInterface := asyncstorageinterface.New(storage, *cfg.StorageInterfaceOpts)
	storageInterface.Start(ctx, output)

	queueManager := queuemanager.New(multiEncoder)
	queueManager.Options = *cfg.QueueOpts
	queueManager.Workers["csv"] = multiEncoder.DecodeCSV
	queueManager.Workers["json"] = multiEncoder.DecodeJSONBatch
	queueManager.Start(ctx)

	cacheManager := cachemanager.New()

	proc := requestprocessor.New(storage, schemaManager, multiEncoder, analyzer, cacheManager)

	client := client.New(schemaManager, storage, queueManager, notificationManager, cacheManager)
	client.Options = cfg.ClientOpts
	client.Start(context.Background())

	api := api.New(storage, proc, schemaManager, queueManager, userManager)
	api.Options = cfg.APIOpts
	api.Init()
	api.Run()
}
