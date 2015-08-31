package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	"github.com/bkeroack/travel"
)

const (
	rootTreePath = "root_tree.json"
)

func getRootTree() (map[string]interface{}, error) {
	var v map[string]interface{}
	d, err := ioutil.ReadFile(rootTreePath)
	if err != nil {
		return map[string]interface{}{}, err
	}
	err = json.Unmarshal(d, &v)
	if err != nil {
		return map[string]interface{}{}, err
	}
	return v, nil
}

func saveRootTree(rt map[string]interface{}) error {
	b, err := json.Marshal(rt)
	if err != nil {
		return err
	}
	f, err := os.Create(rootTreePath)
	if err != nil {
		return err
	}
	_, err = f.Write(b)
	return err
}

//primaryHandler handler runs for every valid request
func primaryHandler(w http.ResponseWriter, r *http.Request, c *travel.Context) {
	saveRt := func() bool {
		err := saveRootTree(c.RootTree)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error saving root tree: %v", err), http.StatusInternalServerError)
		}
		return err == nil
	}

	jsonOutput := func(val interface{}) {
		b, err := json.Marshal(val)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error serializing output: %v", err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	}

	checkEmpty := func() bool {
		if len(c.Path) == 1 && c.Path[0] == "" {
			http.Error(w, "Bad Request: key is required", http.StatusBadRequest)
			return false
		}
		return true
	}

	switch r.Method {
	case "GET":
		jsonOutput(c.CurrentObj) // CurrentObj is the object returned after full traveral; eg '/foo/bar': CurrentObj = root_tree["foo"]["bar"]
	case "PUT":
		if !checkEmpty() {
			return
		}
		d := json.NewDecoder(r.Body)
		var b interface{}
		err := d.Decode(&b)
		if err != nil {
			http.Error(w, fmt.Sprintf("Could not serialize request body: %v", err), http.StatusBadRequest)
			return
		}
		var co map[string]interface{}
		if len(c.Subpath) == 0 { // key exists
			po, wberr := c.WalkBack(1) // c.CurrentObj is the *value*, so we have to walk back one
			if wberr != nil {
				http.Error(w, wberr.Error(), http.StatusInternalServerError)
			}
			co = po
		} else { // key doesn't exist yet
			co = c.CurrentObj.(map[string]interface{})
		}
		k := c.Path[len(c.Path)-1]
		co[k] = b //maps are reference types, so a modification to CurrentObj is reflected in RootTree
		if saveRt() {
			log.Printf("Write: key: %v ; value: %v\n", k, b)
			w.Header().Set("Location", fmt.Sprintf("http://%v/%v", r.Host, r.URL.Path))
			jsonOutput(map[string]string{
				"success": "value written",
			})
			return
		}
		http.Error(w, "Error saving value", http.StatusInternalServerError)
		return
	case "DELETE":
		if !checkEmpty() {
			return
		}
		po, err := c.WalkBack(1) // We need to get the object one node up in the root tree, so we can delete the current object
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		k := c.Path[len(c.Path)-1]
		delete(po, k) // delete the node from the last path token, which must exist otherwise the req would have 404ed
		if saveRt() {
			log.Printf("Delete: key: %v\n", k)
			jsonOutput(map[string]string{
				"success": "value deleted",
			})
			return
		}
		http.Error(w, "Error deleting value", http.StatusInternalServerError)
		return
	default:
		w.Header().Set("Accepts", "GET,PUT,DELETE")
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

//errorHandler Travel runs this in the event of error conditions (including 404s, etc)
func errorHandler(w http.ResponseWriter, r *http.Request, err travel.TraversalError) {
	http.Error(w, err.Error(), err.Code())
}

func main() {
	hm := map[string]travel.TravelHandler{
		"": primaryHandler,
	}
	options := travel.TravelOptions{
		StrictTraversal:   true,
		UseDefaultHandler: false, // DefaultHandler is empty string by default (zero value for string)
		SubpathMaxLength: map[string]int{
			"GET":    0,
			"PUT":    1,
			"DELETE": 0,
		},
	}
	r, err := travel.NewRouter(getRootTree, hm, errorHandler, &options)
	if err != nil {
		log.Fatalf("Error creating Travel router: %v\n", err)
	}
	http.Handle("/", r)
	log.Printf("Listening on port 8000")
	http.ListenAndServe("0.0.0.0:8000", nil)
}
