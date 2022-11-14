package componentanalyzer

import (
	"context"
	"encoding/json"
	"errors"
	"log"

	"golang.org/x/sync/errgroup"
)

type analyzer struct {
	encoder Encoder
	schemaManager SchemaManager
	input chan []string
	deleteRequestInput chan []byte
	group *errgroup.Group
}

type Encoder interface {
	EncodeJSON(data, params map[string]string) ([]byte, error)
}

type SchemaManager interface {
	GetNames(id string) ([]string, int, error)
	GetParams(id string) (map[string]string, error)
}

type worker struct {
	data []string
	deleteData []byte
	output chan []interface{}
	encoder Encoder
	schemaManager SchemaManager
}

func New(encoder Encoder, schemaManager SchemaManager) *analyzer {
	return &analyzer{
		encoder: encoder,
		schemaManager: schemaManager,
		input: make(chan []string, 5),
		deleteRequestInput: make(chan []byte, 5),
		group: &errgroup.Group{},
	}
}

//Starts analyzer with specific output chan from storage interface
func(a *analyzer) Start(ctx context.Context, output chan []interface{}) {
	go func () {
	loop:
	for {
		select {
		case delData := <- a.deleteRequestInput:
			wk := worker{deleteData: delData, output: output, schemaManager: a.schemaManager, encoder: a.encoder}
			a.group.Go(wk.handleDelete)
		case data := <- a.input:
			wk := worker{data: data, output: output, schemaManager: a.schemaManager, encoder: a.encoder }
			a.group.Go(wk.do)
		case <- ctx.Done():
			break loop
		}
	}
		err := a.group.Wait()
		if err != nil {
			log.Println(err.Error())
		}
	}()
}

//do: converts component record to row for db
func(w *worker) do() error {
	log.Println("worker got data from", w.data[0])
	if len(w.data[1:]) == 1 {
			return errors.New("isnt a proper record")
	}

	outputData := make([]interface{}, 4)
	jsonMap := make(map[string]string)

	names, nameField, err := w.schemaManager.GetNames(w.data[0])
	fields := w.data[1:]
	if err != nil {
		return err
	}
	for i, column := range names {
		if i >= len(fields) {
			break
		}
		jsonMap[column] = fields[i]
	}
	params, err := w.schemaManager.GetParams(w.data[0])
	if err != nil {
		return err
	}
	component, err := w.encoder.EncodeJSON(jsonMap, params)
	if err != nil {
		return err
	}

	outputData[0] = w.data[0] //id column id db
	outputData[1] = w.data[nameField] //component name
	outputData[2] = component //component record itself along with params
	outputData[3] = true //tracking: TRUE means that component should be tracked

	w.output <- outputData

	return nil
}

//Handle delete: convertes delete req to row to append to db
func(w *worker) handleDelete() error {
	id := string(w.deleteData[len(w.deleteData)-8:])
	log.Println("handling delete req for", id)

	jsonMap := make(map[string]string)
	err := json.Unmarshal(w.deleteData[:len(w.deleteData)-8], &jsonMap)
	if err != nil {
		log.Println(err.Error())
		return err
	}

	params, err := w.schemaManager.GetParams(id)
	if err != nil {
		return err
	}

	body, err := w.encoder.EncodeJSON(jsonMap, params)
	if err != nil {
		log.Println(err.Error())
		return err
	}

	component := make([]interface{}, 4)
	component[0] = id
	component[1] = jsonMap["part name"]
	component[2] = body
	component[3] = false
	w.output <- component

	return nil
}

//Get input: returns input channel
func(a *analyzer) GetInput() chan []string {
	return a.input
}

//Get delete input: returns input for delete rows
func(a *analyzer) GetDeleteInput() chan []byte {
	return a.deleteRequestInput
}
