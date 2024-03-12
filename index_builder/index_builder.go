package main

import (
  "log"
  "bytes"
  "context"
  "sync"
  "os"
  "fmt"
  "encoding/json"
  "database/sql"
  _ "github.com/mattn/go-sqlite3"
  "github.com/elastic/go-elasticsearch/v8"
  "github.com/elastic/go-elasticsearch/v8/esapi"
)

const indexName = "filings_2024_03_11"
const dbPath = "filings-2024-03-11.sqlite3"
const selectString = `
  SELECT 
    companies.ticker, 
    companies.name, 
    coalesce(companies.index_membership, ''), 
    filings.accession_number, 
    filings.filed_date, 
    filings.link_10k, 
    item1.contents AS item1, 
    coalesce(item1a.contents, '') AS item1a
  FROM companies 
  JOIN filings ON companies.ticker=filings.ticker
  JOIN item1 ON filings.accession_number=item1.accession_number
  LEFT JOIN item1a ON filings.accession_number=item1a.accession_number
  WHERE substr(filings.filed_date,1,4)>='2005'`

type QueryResult struct {
  Ticker     string 
  Name       string
  StockIndex string
  Filed      string
  Url        string
  Item1      string `json:"1. Business"`
  Item1a     string `json:"1A. Risk Factors"`
}


var es *elasticsearch.Client

func clientInit() {
  var err error

  cfg := elasticsearch.Config {
    Addresses: []string {
      "https://localhost:9200",
    },
    Username: "elastic",
    Password: os.Getenv("ES_PASS"),
    CertificateFingerprint: os.Getenv("ES_CERTFP"),
  }

  es, err = elasticsearch.NewClient(cfg)
  if err != nil {
    log.Fatalf("Error creating client: %s", err)
  }
}

// see https://github.com/elastic/go-elasticsearch/blob/main/_examples/bulk/default.go
func bulkInsert(ids []string, qrs []QueryResult, wg *sync.WaitGroup) {
  defer wg.Done()

  var payload []byte

  for i, qr := range qrs {
    // Build the request body.
    data, err := json.Marshal(qr)
    if err != nil {
      log.Fatalf("Error marshaling document: %s", err)
    }

    // prepare metadata
    meta := []byte(fmt.Sprintf(`{ "index" : { "_id" : "%s" } }%s`, ids[i], "\n"))

    // add to payload
    payload = append(payload, meta...)
    payload = append(payload, data...)
    payload = append(payload, "\n"...) // bulk api expects newline separated documents

  }

  res, err := es.Bulk(bytes.NewReader(payload), es.Bulk.WithIndex(indexName))
  if err != nil {
    log.Fatalf("Error getting response: %s", err)
  }
  if res.IsError() {
    log.Fatalf("[%s] Error in bulk indexing starting with ID=%s", res.Status(), ids[0])
  }
  res.Body.Close()
}

func main() {
	var (
    res *esapi.Response
    err error
    row *sql.Rows
    wg sync.WaitGroup
    ids []string
    qrs []QueryResult
	)

  // open sql database
  db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Fatalf("Error opening database  : %s", err)
	}

  // initialize elasticsearch client and recreate index
  clientInit()
  res, err = esapi.IndicesExistsRequest{
    Index: []string{indexName},
  }.Do(context.Background(), es)
  if err != nil {
		log.Fatalf("Error checking if index exists: %s", err)
	}
  defer res.Body.Close()
  if res.StatusCode == 200 {
    // index already exists, delete
    res, err = es.Indices.Delete([]string{indexName})
    if err != nil {
      log.Fatalf("Error deleting index: %s", err)
    }
    if res.IsError() {
      log.Fatalf("res error deleting index: %s", res.Status())
    }
    res.Body.Close()
  }
  res, err = es.Indices.Create(indexName)
  if err != nil {
		log.Fatalf("Error creating index: %s", err)
	}
  if res.IsError() {
		log.Fatalf("res error creating index: %s", res.Status())
	}
  res.Body.Close()

	// query db and index documents
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
    var qr QueryResult
    var id string // use accession_number for id
    err = row.Scan(&qr.Ticker, &qr.Name, &qr.StockIndex, &id, &qr.Filed, 
                   &qr.Url, &qr.Item1, &qr.Item1a) 
    if err != nil {
      log.Fatalf("Error scanning row: %s", err)
    }

    ids = append(ids, id)
    qrs = append(qrs, qr)

    if i % 100 == 0 {
      log.Println(i)
      wg.Add(1)
      bulkInsert(ids, qrs, &wg)
      ids = nil
      qrs = nil
    }

	}
  // add any leftover
  if len(ids) != 0 {
    wg.Add(1)
    bulkInsert(ids, qrs, &wg)
  }

  log.Println(i)
  wg.Wait()
}
