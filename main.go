package main

import (
	"flag"
	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

func getPages(indexPage, sessionCookieKey, sessionCookieVal string) []string {

	pages := make([]string, 0)

	req, err := http.NewRequest("GET", indexPage, nil)
	if err != nil {
		log.Fatal(err)
	}
	req.AddCookie(&http.Cookie{
		Name:  sessionCookieKey,
		Value: sessionCookieVal,
	})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		log.Fatal("HTTP status code != 200")
	}
	htmlRoot, err := html.Parse(resp.Body)
	if err != nil {
		log.Fatal(err)
	}

	if htmlRoot == nil {
		log.Fatal("html root does not exist")
	}

	for h := htmlRoot.FirstChild; h != nil; h = h.NextSibling {
		if h.Type == html.DoctypeNode {
			continue
		}
		if h.Data == "html" {
			htmlRoot = h
			break
		}
	}

	var body *html.Node

	for h := htmlRoot.FirstChild; h != nil; h = h.NextSibling {
		if h.Data == "body" {
			body = h
			break
		}
	}

	if body == nil {
		log.Fatal("Body is nil")
	}

	var pageDiv *html.Node
	for h := body.FirstChild; h != nil; h = h.NextSibling {
		if h.Data == "div" && hasAttr(h.Attr, "id", "page") {
			pageDiv = h
			break
		}
	}
	if pageDiv == nil {
		log.Fatal("Did not find page div")
	}

	var contentDiv *html.Node
	for h := pageDiv.FirstChild; h != nil; h = h.NextSibling {
		if h.Data == "div" && hasAttr(h.Attr, "id", "content") {
			contentDiv = h
			break
		}
	}
	if contentDiv == nil {
		log.Fatal("Did not find content div")
	}

	var firstPage *html.Node
	for h := contentDiv.FirstChild; h != nil; h = h.NextSibling {
		if h.Data == "p" && len(h.Attr) == 0 {
			firstPage = h.NextSibling.NextSibling
			break
		}
	}

	for h := firstPage; h != nil; h = h.NextSibling {
		if h.Data == "ul" && h.Type == html.ElementNode {
			pages = append(pages, h.FirstChild.FirstChild.Attr[0].Val)
		}
	}
	return pages
}

func hasAttr(attributes []html.Attribute, key, val string) bool {
	for _, a := range attributes {
		if a.Key == key && a.Val == val {
			return true
		}
	}
	return false
}

func getPageSrc(pagePath, sessionCookieKey, sessionCookieVal string) string {
	pagePath = strings.TrimPrefix(pagePath, "/")
	pagePath = pagePath[strings.Index(pagePath, "/"):]
	req, err := http.NewRequest("GET", "https://wiki.stuve.uni-ulm.de/carolo-cup/action/edit"+pagePath+"?action=edit&editor=text", nil)
	if err != nil {
		log.Fatal(err)
	}
	req.AddCookie(&http.Cookie{
		Name:  sessionCookieKey,
		Value: sessionCookieVal,
	})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	if resp.StatusCode != 200 {
		log.Fatal("Requesting " + req.URL.String() + " resulted in status " + strconv.Itoa(resp.StatusCode))
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		log.Fatal(err)
	}

	nodes := doc.Find("#editor-textarea").Nodes
	if len(nodes) < 1 {
		log.Println("Page " + pagePath + " is not editable")
		return ""
	}

	textArea := nodes[0]

	return textArea.FirstChild.Data
}

func main() {
	sessionCookie := flag.String("sessionCookie", "", "Session Cookie")
	flag.Parse()
	pages := getPages("https://wiki.stuve.uni-ulm.de/carolo-cup/TitelIndex",
		"MOIN_SESSION_443_ROOT_carolo-cup", *sessionCookie)
	for _, path := range pages {
		log.Printf("Handling page \"%s\"\n", path)
		page := getPageSrc(path, "MOIN_SESSION_443_ROOT_carolo-cup", *sessionCookie)
		filepath := strings.TrimPrefix(path, "/")
		log.Printf("Filepath %s\n", filepath)
		if strings.Contains(filepath, "/") {
			dir := filepath[:strings.LastIndex(filepath, "/")]
			makeDir(dir)
		}

		// This block checks the case if the directory was created before, and we are now writing the index
		s, err := os.Stat(filepath)
		if err == nil {
			if s.IsDir() {
				log.Printf("%s is a dir, new path is %s", filepath, filepath+"/index")
				filepath = filepath + "/index"
			}
		}

		err = ioutil.WriteFile(filepath, []byte(page), 0o644)
		if err != nil {
			log.Fatal(err)
		}
		time.Sleep(10 * time.Second)
	}
}

func makeDir(path string) {
	log.Printf("Ensuring %s is a directory\n", path)
	lastExisting := path
	for !pathExists(lastExisting) {
		lastExisting = lastExisting[:strings.LastIndex(lastExisting, "/")]
	}

	log.Printf("The last existing file in the path is %s\n", lastExisting)

	s, err := os.Stat(lastExisting)
	if err != nil {
		log.Fatal(err)
	}

	if !s.IsDir() {
		// The last existing part in path is not a directory.
		// Create a directory in its place and move the file to directory/index
		log.Printf("%s is not a directory, moving file.\n", lastExisting)
		err := os.Rename(lastExisting, lastExisting+"-index")
		if err != nil {
			log.Fatal(err)
		}
		err = os.MkdirAll(lastExisting, 0o755)
		if err != nil {
			log.Fatal(err)
		}
		err = os.Rename(lastExisting+"-index", lastExisting+"/index")
		if err != nil {
			log.Fatal(err)
		}
	}
	log.Printf("%s is now a directory.\n", lastExisting)

	err = os.MkdirAll(path, 0o755)
	if err != nil {
		log.Fatal(err)
	}
}

func pathExists(path string) bool {
	if _, err := os.Stat(path); err == nil {
		return true
	}
	return false
}
