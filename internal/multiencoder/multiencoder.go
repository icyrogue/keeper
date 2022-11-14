package multiencoder

import (
	"bufio"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"log"
	"strings"
)

type multiEncoder struct {
	Output                chan []string
	StorageInterfaceInput chan []interface{}

	schemaManager SchemaManager
}

type item struct {
	Component map[string]string `json:"component"`
	Params    map[string]string `json:"parameters"`
}

type SchemaManager interface {
	CompareFieldNames(id string, names []string) ([]string, error)
	GetParams(id string) (map[string]string, error)
}

func New(schemaManager SchemaManager) *multiEncoder {
	return &multiEncoder{schemaManager: schemaManager}
}

// EncodeJSON: packs compoenent data and params as JSON for db column
func (m *multiEncoder) EncodeJSON(newComponent, params map[string]string) ([]byte, error) {
	log.Println("started encoding JSON")
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetIndent("", " ")
	component := item{Component: newComponent, Params: params}
	if err := encoder.Encode(component); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

// DecodeCSV: reads scv and passes it to worker
func (m *multiEncoder) DecodeCSV(ctx context.Context, id string, data io.Reader) error {
	reader := csv.NewReader(data)
	row, err := reader.Read()
	if err != nil {
		return err
	}
	if len(row) == 1 {
		reader.Comma = ';'
		row = strings.Split(row[0], ";")
		if len(row) == 1 {
			return errors.New("unable to read csv")
		}
	}
	_, err = m.schemaManager.CompareFieldNames(id, row)
	if err != nil {
		return err
	}
loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		default:
			row, err := reader.Read()
			if err != nil {
				if err == io.EOF {
					return nil
				}
				return err
			}
			//ID should always be first in array so worker can find it
			m.Output <- append([]string{id}, row...)
		}
	}
	//TODO: add some graceful shutdown roitine
	return nil
}

// Decode JSON batch: converts JSON data into rows of components
func (m *multiEncoder) DecodeJSONBatch(ctx context.Context, id string, data io.Reader) error {
	var prevLength int

	split := func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		if atEOF && len(data) == 0 {
			return 0, nil, nil
		}
		if i := bytes.IndexByte(data, '}'); i >= 0 {
			i++
			return i + 1, data[0:i], nil
		}
		if atEOF {
			return len(data), data, nil
		}
		return 0, nil, nil
	}
	scanner := bufio.NewScanner(data)
	scanner.Split(split)

loop:
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			break loop
		default:
			jsonMap := make(map[string]string)
			body := scanner.Bytes()
			if prevLength == 0 {
				i := bytes.IndexByte(body, '[')
				log.Println(i)
				body = body[i+1:]
			}
			if err := json.Unmarshal(body, &jsonMap); err != nil {
				log.Println(err.Error())
				continue
			}

			if l := len(jsonMap); l > prevLength {
				names := make([]string, l)
				for name := range jsonMap {
					l--
					names[l] = name
				}
				prevLength = l
				_, err := m.schemaManager.CompareFieldNames(id, names[1:])
				if err != nil {
					return err
				}
			}

			component := make([]interface{}, 4)
			name, fd := jsonMap["part name"]
			if !fd {
				log.Println("unable to determine compoennt name")
				continue
			}

			params, err := m.schemaManager.GetParams(id)
			if err != nil {
				log.Println("couldnt get schema for", id)
				return errors.New("no schema for id")
			}

			body, err = m.EncodeJSON(jsonMap, params)
			if err != nil {
				log.Println("unable to encode elemnt of json batch")
				continue
			}

			component[0] = id   //id column id db
			component[1] = name //component name
			component[2] = body //component record itself along with params
			component[3] = true //tracking: TRUE means that component should be tracked

			m.StorageInterfaceInput <- component
			log.Println("got to the end of JSON batch for ID", id)
		}
	}
	//TODO: add some graceful shutdown routine
	return nil
}
