package main

import (
	"encoding/json"
	"log"
	"os"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/stretchr/testify/assert"
)

const (
	addr = "http://localhost:8080"
)

type user struct {
	email       string
	token       string
	id          string
	cmpToDelete []byte
}

type testComponent struct {
	Name     string `json:"part name"`
	Position string `json:"position"`
	Amount   string `json:"amount"`
}

func Test_Register(t *testing.T) {
	user := user{
		email: "iwannagocollectleafs@bebe.bobo",
	}
	client := resty.New()
	t.Run("login", func(t *testing.T) {
		go main()
		time.Sleep(5 * time.Second)

		resp, err := client.R().Get(addr + "/api/ping")
		assert.NoError(t, err)
		assert.False(t, resp.IsError())

		resp, err = client.R().
			SetBody(user.email).
			Post(addr + "/api/login")
		assert.NoError(t, err)
		assert.True(t, resp.IsSuccess(), string(resp.Body()))
		assert.NotNil(t, resp.Body())
		user.token = string(resp.Body())
	})

	t.Run("Add and create", func(t *testing.T) {
		resp, err := client.R().
			SetHeader("Token", user.token).
			Post(addr + "/api/list")
		assert.NoError(t, err)
		assert.True(t, resp.IsSuccess(), string(resp.Body()))
		id := string(resp.Body())
		id = id[len(id)-8:]
		user.id = id
		log.Println(id)
		component := testComponent{
			Name:     "TL072",
			Position: "IC1",
			Amount:   "2",
		}
		body, err := json.Marshal(&component)
		if err != nil {
			t.Fatal(err.Error())
		}
		resp, err = client.R().EnableTrace().SetBody(body).SetHeader("Token", user.token).SetPathParam("id", id).Post(addr + "/api/list/{id}")
		assert.NoError(t, err)
		log.Println(resp.Request.URL, user.id)
		assert.True(t, resp.IsSuccess(), resp)
		user.cmpToDelete = body
	})

	t.Run("test Delete", func(t *testing.T) {
		resp, err := client.R().EnableTrace().SetBody(user.cmpToDelete).SetHeader("Token", user.token).SetPathParam("id", user.id).Put(addr + "/api/list/{id}")
		assert.NoError(t, err)
		log.Println(resp.Request.URL, user.id)
		assert.True(t, resp.IsSuccess(), resp)
		time.Sleep(50 * time.Second)

	})

	t.Run("test batch", func(t *testing.T) {
		components := []testComponent{{Name: "CD4020", Position: "IC1", Amount: "2"},
			{Name: "MPC2324", Position: "IC14", Amount: "2"},
			{Name: "PIC16f1", Position: "IC8", Amount: "1"}}
		body, err := json.Marshal(&components)
		if err != nil {
			t.Fatal(err.Error())
		}
		resp, err := client.R().SetBody(body).SetHeader("Token", user.token).SetPathParam("id", user.id).Post(addr + "/api/list/{id}/batch")
		if err != nil {
			t.Fatal(err.Error())
		}
		assert.True(t, resp.IsSuccess())
		time.Sleep(50 * time.Second)
	})

	t.Run("test cache", func(t *testing.T) {
		resp, err := client.R().SetHeader("Token", user.token).SetPathParam("id", user.id).SetPathParam("name", "TL072").Get(addr + "/api/list/{id}/{name}")
		if err != nil {
			t.Fatal(err.Error())
		}
		assert.True(t, resp.IsSuccess())
		log.Println(string(resp.Body()))
	})

	t.Run("test csv", func(t *testing.T) {
		body, err := os.Open(`..//testCSV.csv`)
		if err != nil {
			t.Fatal(err.Error())
		}
		resp, err := client.R().SetBody(body).SetHeader("Token", user.token).SetPathParam("id", user.id).Post(addr + "/api/list/{id}/bom")
		if err != nil {
			t.Fatal(err.Error())
		}
		assert.True(t, resp.IsSuccess())
		time.Sleep(50 * time.Second)
	})
}
