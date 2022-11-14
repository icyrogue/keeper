package jsonmodels

type JSONResponse struct {
	Rows      []Row     `json:"rows"`
	Stockdata Stockdata `json:"stockdata"`
}

type Row struct {
	Name         string          `json:"part"`
	URL          string          `json:"url"`
	Manufacturer string          `json:"mfg"`
	Stock        string          `json:"stock"`
	Price        [][]interface{} `json:"price"`
}

type Stockdata struct {
	Title  string `json:"title"`
	City   string `json:"city"`
	Site   string `json:"site"`
	Email  string `json:"contact_email"`
	Limits string `json:"min_order"`
	//	Country string `json:"country"`

}
