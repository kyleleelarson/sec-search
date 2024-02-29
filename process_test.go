// unittests for some helper functions
package main

import (
  "io"
  "testing"
  "strings"
  "strconv"
  "net/http"
  "net/http/httptest"
)

// test processYear function from search.go
func TestProcessYear(t *testing.T) {
  year, expectedLower, expectedUpper := "2021", "2020-12-31", "2022-01-01"
  lower, upper := processYear(year)
  if lower != expectedLower || upper != expectedUpper {
    t.Fatalf(`processYear("%s") = %s, %s, expected %s, %s.`, 
      year, lower, upper, expectedLower, expectedUpper)
  }

  year, expectedLower, expectedUpper = "2009", "2008-12-31", "2010-01-01"
  lower, upper = processYear(year)
  if lower != expectedLower || upper != expectedUpper {
    t.Fatalf(`processYear("%s") = %s, %s, expected %s, %s.`, 
      year, lower, upper, expectedLower, expectedUpper)
  }

  // test year out of range or invalid input
  lowerBoundStr :=  strconv.Itoa(yearLowerBound) + "-12-31"
  upperBoundStr :=  strconv.Itoa(yearUpperBound) + "-01-01"

  year, expectedLower, expectedUpper = "1992", lowerBoundStr, upperBoundStr
  lower, upper = processYear(year)
  if lower != expectedLower || upper != expectedUpper {
    t.Fatalf(`processYear("%s") = %s, %s, expected %s, %s.`, 
      year, lower, upper, expectedLower, expectedUpper)
  }

  year, expectedLower, expectedUpper = "2012-03-17", lowerBoundStr, upperBoundStr
  lower, upper = processYear(year)
  if lower != expectedLower || upper != expectedUpper {
    t.Fatalf(`processYear("%s") = %s, %s, expected %s, %s.`, 
      year, lower, upper, expectedLower, expectedUpper)
  }

}

// test processParameters funcion from server.go
var processedP Parameters

// processParameters returns a handler function build from the following function signature
func testHandler(w http.ResponseWriter, r *http.Request, p *Parameters) {
  // set processedP parameters with those passed in
  processedP.searchTerm = p.searchTerm
  processedP.stockIndex = p.stockIndex
  processedP.section    = p.section
  processedP.year       = p.year
  processedP.page       = p.page
}

func TestProcessParameters(t *testing.T) {
  searchTerm := "artificial intelligence"
  page, _ := strconv.Atoi(defaultPage)
  w := httptest.NewRecorder()
  handler := processParameters(testHandler)

  // test all defaults
  reqStr := "/search?searchterm=" + strings.Replace(searchTerm, " ", "+", -1)
  expectedP := Parameters{searchTerm, defaultStockIndex, defaultSection, defaultYear, page}
  req := httptest.NewRequest(http.MethodGet, reqStr, nil)
  handler(w, req)
  if processedP != expectedP {
    t.Fatalf("processedP = %v, expectedP %v.", processedP, expectedP)
  }

  // test custom inputs
  expectedP = Parameters{searchTerm, "RUSSELL2000", "Item1a", "2012", 2}
  pageStr := strconv.Itoa(expectedP.page)
  reqStr = reqStr + "&stockindex=" + expectedP.stockIndex + "&section=" + expectedP.section + 
           "&year=" + expectedP.year + "&p=" + pageStr 
  req = httptest.NewRequest(http.MethodGet, reqStr, nil)
  handler(w, req)
  if processedP != expectedP {
    t.Fatalf("processedP = %v, expectedP %v.", processedP, expectedP)
  }

  // test invalid page
  reqStr = reqStr[:len(reqStr)-1] + "invalidpage" 
  req = httptest.NewRequest(http.MethodGet, reqStr, nil)
  handler(w, req)
  res := w.Result()
  defer res.Body.Close()
  if res.StatusCode != http.StatusBadRequest {
    t.Fatalf("status code %v, expected %v.", res.StatusCode, http.StatusBadRequest)
  }
  b, _ := io.ReadAll(res.Body)
  body := strings.Join(strings.Fields(string(b)), " ") // remove extra whitespace
  expectedBody := "invalid page parameter"
  if body != expectedBody {
    t.Fatalf("body %v, expected %v.", body, expectedBody)
  }
}
