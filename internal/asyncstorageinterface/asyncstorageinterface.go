package asyncstorageinterface

import (
	"context"
	"log"
	"time"
)



type storageInterface struct {
	data [][]interface{}
	storage Storage
	options *Options
}

type Options struct {
	MaxWaitTime int
	MaxBufferLength int
}

type Storage interface {
	AddItem(ctx context.Context, args [][]interface{}) error
}

func New(storage Storage, options Options) *storageInterface {
	return &storageInterface{storage: storage,
		options: &options,
		data: [][]interface{}{},
		}
}

//Starts storage interface with input from analyzer
func (si *storageInterface) Start(ctx context.Context, input chan []interface{}) {
	go func (){
	log.Println("started storage interface")
		timer := time.AfterFunc(time.Duration(si.options.MaxWaitTime)*time.Second, si.appendToDB)
		defer timer.Stop()
		loop:
	for {
		select {
		case <- ctx.Done():
			break loop
		case v := <- input:
			log.Printf("storage got %s, %s, %s", v[0], v[1], v[2])
			timer.Reset(time.Duration(si.options.MaxWaitTime)*time.Second)
			si.data = append(si.data, v)
			if len(si.data) > si.options.MaxBufferLength {
				si.appendToDB()
			}
		}
	}
		timer.Stop()
	}()
}

//appendToDB: appends every item from storage interface to db
func (si *storageInterface) appendToDB() {
	if len(si.data) == 0 {
		return
	}
	if err := si.storage.AddItem(context.Background(), si.data); err != nil {
		log.Println(err.Error())
	}
	si.data = [][]interface{}{}
	log.Println("appended to db")
}
