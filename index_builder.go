package main

import (
  "log"
  "bytes"
  "sync"
  "os"
  "context"
  "strconv"
  "encoding/json"
  "database/sql"
  _ "github.com/mattn/go-sqlite3"
  "github.com/elastic/go-elasticsearch/v8"
  "github.com/elastic/go-elasticsearch/v8/esapi"
)

const dbPath = "filings-2024-02-07.sqlite3"
const selectString = `
  SELECT companies.ticker, companies.name, companies.industry, 
  companies.index_membership, filings.accession_number, filings.filed_date, filings.cik,
  filings.state_of_incorporation, filings.fiscal_year_end, item1.contents, item1.epic
  FROM companies 
  JOIN filings ON companies.ticker=filings.ticker
  JOIN item1 ON filings.accession_number=item1.accession_number
  LIMIT 3000`

type QueryResult struct {
  Ticker, Name, Industry, Index, AccNum, Filed, Cik, IncorpSt, YearEnd, Contents, Epic sql.NullString
}


var client *elasticsearch.Client

func client_init() {
  var err error

  cfg := elasticsearch.Config {
    Addresses: []string {
      "https://localhost:9200",
    },
    Username: "elastic",
    Password: os.Getenv("ES_PASS"),
    CertificateFingerprint: os.Getenv("ES_CERTFP"),
  }

  client, err = elasticsearch.NewClient(cfg)
  if err != nil {
    log.Fatalf("Error creating client: %s", err)
  }
}

func main() {
	var (
    row *sql.Rows
		wg sync.WaitGroup
	)

  // open sql database
  db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Fatalf("Error opening database  : %s", err)
	}

  // initialize elasticsearch client
  client_init()
  // delete index
  client.Indices.Delete([]string{"filings"})

	// 1. Get cluster info
	res, err := client.Info()
	if err != nil {
		log.Fatalf("Error getting response: %s", err)
	}
	defer res.Body.Close()
	// Check response status
	if res.IsError() {
		log.Fatalf("Error: %s", res.String())
	}


	// 2. query db and index documents concurrently
  selectSt, err := db.Prepare(selectString)
	if err != nil {
		log.Fatalf("Error preparing statement: %s", err)
	}
  row, err = selectSt.Query()
	if err != nil {
		log.Fatalf("Error querying database: %s", err)
	}

  i := 0
	for row.Next() {
    i+=1
		wg.Add(1)
    var qr QueryResult
    err = row.Scan(&qr.Ticker, &qr.Name, &qr.Industry, &qr.Index, &qr.AccNum, &qr.Filed, 
                   &qr.Cik, &qr.IncorpSt, &qr.YearEnd, &qr.Contents, &qr.Epic) 
    if err != nil {
      log.Fatalf("Error scanning row: %s", err)
    }

		go func(i int, qr QueryResult) {
			defer wg.Done()

			// Build the request body.
			data, err := json.Marshal(qr)
			if err != nil {
				log.Fatalf("Error marshaling document: %s", err)
			}

			// Set up the request object.
			req := esapi.IndexRequest{
				Index:      "filings",
        DocumentID: strconv.Itoa(i) + ":item1",
				Body:       bytes.NewReader(data),
				Refresh:    "true",
			}

			// Perform the request with the client.
			res, err := req.Do(context.Background(), client)
			if err != nil {
				log.Fatalf("Error getting response: %s", err)
			}
			if res.IsError() {
				log.Printf("[%s] Error indexing document ID=%d", res.Status(), i)
			}
      res.Body.Close()
		}(i, qr)
	}
	wg.Wait()
  log.Println(i)
}
