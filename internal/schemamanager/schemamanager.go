package schemamanager

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"sync"
)

type schema struct {
	ID             string `json:"id"`
	NameField      string `json:"nameField"`
	nameFieldPos   int
	fieldNames     []string
	FieldsAsString string `json:"fieldNames"`
	Region         string `json:"region"`
	MinAmount      string `json:"minimumAmount"`
}

type schemaManager struct {
	data    map[string]schema
	mtx     sync.RWMutex
	storage Storage
	Options *Options
}

type Options struct {
	Filepath string
}

const (
	defaultRegion    = "1"
	defaultNameField = "part name"
	defaultMinAmaunt = "100"
)

type Storage interface {
	SyncSchemas(ctx context.Context) ([]byte, error)
}

// New: returns new schema Manager
func New(storage Storage) *schemaManager {
	return &schemaManager{storage: storage, data: make(map[string]schema)}
}

func (sm *schemaManager) Init() error {
	sm.mtx.Lock()
	defer sm.mtx.Unlock()

	body, err := sm.storage.SyncSchemas(context.Background())
	if err != nil {
		return err
	}
	if len(body) < 5 {
		return nil
	}
	var schemas []schema
	if err = json.Unmarshal(body, &schemas); err != nil {
		return err
	}
	for _, sc := range schemas {
		sc.fieldNames = strings.Split(sc.FieldsAsString, ", ")
		sm.data[sc.ID] = sc
	}
	log.Println("recovered schemas of count", len(sm.data))
	return nil
}

// NewSchema: deliberately create new schema with spacific ID
func (sm *schemaManager) NewSchema(id string) error {
	sm.mtx.Lock()
	defer sm.mtx.Unlock()

	if _, fd := sm.data[id]; fd {
		return errors.New("schema for list with such ID already exists")
	}
	sm.data[id] = schema{}
	return nil
}

// GetSchemaJSON: returns schema by ID in JSON format
func (sm *schemaManager) GetSchemaJSON(id string) ([]byte, error) {
	sm.mtx.RLock()
	defer sm.mtx.RUnlock()

	schema, fd := sm.data[id]
	if !fd {
		return nil, errors.New("no schema for list with such ID")
	}
	log.Println(schema)

	return json.MarshalIndent(schema, "", " ")
}

// CompareFieldNames: compares field names of two schemas, if they differ returns new fields and writes them to curren schema
func (sm *schemaManager) CompareFieldNames(id string, data []string) ([]string, error) {
	sm.mtx.Lock()
	defer sm.mtx.Unlock()

	log.Println("comparing field names for", id)

	sc, fd := sm.data[id]
	if !fd {
		sc.ID = id
		sc.fieldNames = data
		sc.FieldsAsString = strings.Join(data, ", ")
		sc.MinAmount = defaultMinAmaunt
		sc.NameField = defaultNameField
		sc.Region = defaultRegion
		sm.data[id] = sc
		return nil, nil
	}
	if len(strings.Join(data, ", ")) == len(sc.FieldsAsString) {
		return nil, nil
	}
	var newFields []string
	for i, field := range data {
		if strings.Contains(field, sc.FieldsAsString) {
			newFields = append(newFields, field)
		}
		if field == "part name" {
			sc.nameFieldPos = i
		}
	}

	sc.fieldNames = append(sc.fieldNames, newFields...)
	sc.FieldsAsString = strings.Join(sc.fieldNames, ", ")

	sm.data[id] = sc

	return nil, nil
}

// Get names: returns original names of component record, and index field with name of component in this array
func (sm *schemaManager) GetNames(id string) (fields []string, position int, err error) {
	sm.mtx.RLock()
	defer sm.mtx.RUnlock()

	names, fd := sm.data[id]
	if !fd {
		return nil, 0, errors.New("no list with such ID")
	}
	if names.nameFieldPos == 0 {
		for i, field := range names.fieldNames {
			if field == "part name" {
				names.nameFieldPos = i
				return names.fieldNames, names.nameFieldPos, nil
			}
		}
		return nil, 0, errors.New("wasnt able to determine name field position")
	}
	return names.fieldNames, names.nameFieldPos, nil
}

// GetAllIDs: returns every ID for which a schema exists
func (sm *schemaManager) GetAllIDs() []string {
	var output []string
	for key := range sm.data {
		output = append(output, key)
	}
	return output
}

// GetParams: returns parameters as a map for schema
func (sm *schemaManager) GetParams(id string) (map[string]string, error) {
	output := make(map[string]string)
	schema, fd := sm.data[id]
	if !fd {
		return nil, errors.New("no schema with such ID")
	}

	output["id"] = schema.ID
	output["nameField"] = schema.NameField
	output["fieldNames"] = schema.FieldsAsString
	//	output["numField"] = schema.NumField
	output["region"] = schema.Region
	output["minimumAmount"] = schema.MinAmount
	//	output["FieldsAsString"] = schema.FieldsAsString

	return output, nil
}

// SaveSchemaJSON: saves new schema for ID
func (sm *schemaManager) SaveSchemaJSON(id string, data []byte) error {
	var component schema
	_, fd := sm.data[id]
	if !fd {
		return errors.New("no schema for such ID")
	}
	err := json.Unmarshal(data, &component)
	if err != nil {
		return err
	}
	sm.data[id] = component
	return nil
}
