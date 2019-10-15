package main

import (
	"bufio"
	"flag"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var prefix = "carolo-cup"
var mediaSuffixes = []string{".mp4", ".m4v", ".mov", ".webm", ".ogv", ".png", ".jpg", ".jpeg", ".gif", ".bmp"}

var sessionCookie string
var sessionCookieName string
var wikiUrl string

var logLevel int

var (
	Trace   *log.Logger
	Info    *log.Logger
	Warning *log.Logger
	Error   *log.Logger
)

func initLog(traceHandle io.Writer, infoHandle io.Writer, warningHandle io.Writer, errorHandle io.Writer) {
	Trace = log.New(traceHandle,
		"TRACE: ",
		log.Ldate|log.Ltime|log.Lshortfile)

	Info = log.New(infoHandle,
		"INFO: ",
		log.Ldate|log.Ltime|log.Lshortfile)

	Warning = log.New(warningHandle,
		"WARNING: ",
		log.Ldate|log.Ltime|log.Lshortfile)

	Error = log.New(errorHandle,
		"ERROR: ",
		log.Ldate|log.Ltime|log.Lshortfile)
}

func init() {
	flag.StringVar(&sessionCookieName, "sessionCookieName", "MOIN_SESSION_443_ROOT_carolo-cup", "Session Cookie Name")
	flag.StringVar(&sessionCookie, "sessionCookieValue", "", "Session Cookie Value")
	flag.IntVar(&logLevel, "v", 2, "Log Level: 0 = Error, 1 = Warning, 2 = Info, 3 = Trace")
	flag.StringVar(&prefix, "prefix", "carolo-cup", "Name of directory containing wiki files, must be same as wiki name")
	flag.StringVar(&wikiUrl, "url", "https://wiki.stuve.uni-ulm.de", "Wiki URL without trailing slash")
	flag.Parse()
}

func main() {

	switch logLevel {
	case 0:
		// Error
		initLog(ioutil.Discard, ioutil.Discard, ioutil.Discard, os.Stderr)
		break
	case 1:
		// Warning
		initLog(ioutil.Discard, ioutil.Discard, os.Stdout, os.Stderr)
		break
	case 2:
		// Info
		initLog(ioutil.Discard, os.Stdout, os.Stdout, os.Stderr)
		break
	default:
		// Trace
		initLog(os.Stdout, os.Stdout, os.Stdout, os.Stderr)
	}

	err := filepath.Walk(prefix,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".md") {
				return nil
			}
			if strings.Contains(path, ".git") {
				return nil
			}

			Info.Println(path)
			translate(path)
			return nil
		})
	if err != nil {
		Error.Fatal(err)
	}
}

func translate(path string) {
	Info.Println("Translating " + path)
	file, err := os.Open(path)
	if err != nil {
		Error.Fatal(err)
	}

	scanner := bufio.NewScanner(file)
	lines := make([]string, 0)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		Error.Fatal(err)
	}

	headingRegex := regexp.MustCompile(`(=+) (.+) =+`)
	linkRegex := regexp.MustCompile(`\[\[[^]]+]]`)
	inlineCodeRegex := regexp.MustCompile(`{{{(.+?)}}}`)
	attachmentRegex := regexp.MustCompile(`(?m)({{|\[\[)attachment:.+?(}}|]])`)
	codeBlockStartRegex := regexp.MustCompile(`(?m)^\s*{{{(?:#!highlight (.+))?$`)
	codeBlockEndRegex := regexp.MustCompile(`(?m)^\s*}}}\s*$`)
	tableRegex := regexp.MustCompile(`(?m)\|\|(?:<.+?>)?([^|\n]*)`)
	boldRegex := regexp.MustCompile(`(?m)'''\s*([^']+?)\s*'''`)
	italicRegex := regexp.MustCompile(`(?m)''\s*([^']+?)\s*''`)
	strikethroughRegex := regexp.MustCompile(`(?m)--\(\s*([^']+?)\s*\)--`)

	isInCodeBlock := false
	isFirstTableRow := true
	currentBaseIndent := -1
	startComments := true

	for i, _ := range lines {
		// Skip comments at start of file
		if strings.HasPrefix(lines[i], "#") && startComments {
			lines[i] = "#DISCARD"
			continue
		} else {
			startComments = false
		}

		// Fix Code Blocks
		if isInCodeBlock {
			codeBlockEnd := codeBlockEndRegex.FindAllString(lines[i], -1)
			if len(codeBlockEnd) > 0 {
				lines[i] = "~~~"
				isInCodeBlock = false
			}
			continue
		} else {
			codeBlockStart := codeBlockStartRegex.FindAllStringSubmatch(lines[i], -1)
			if len(codeBlockStart) > 0 {
				lines[i] = "~~~"
				if len(codeBlockStart[0]) > 1 {
					lines[i] += codeBlockStart[0][1]
				}
				isInCodeBlock = true
				continue
			}
		}

		// Fix Tables
		tableMatches := tableRegex.FindAllStringSubmatch(lines[i], -1)
		if len(tableMatches) > 0 {

			lines[i] = tableRegex.ReplaceAllString(lines[i], "|$1")
			lines[i] = strings.TrimRight(lines[i], "|") + "|" // Ensure only one "|" at the end

			if isFirstTableRow {
				lines[i] += "#TABLE-" + strconv.Itoa(len(tableMatches)-1)
			}
			isFirstTableRow = false
		} else {
			isFirstTableRow = true
		}

		// Fix weird symbols
		lines[i] = strings.Replace(lines[i], "(./)", "✓", -1)
		lines[i] = strings.Replace(lines[i], "{X}", "✗", -1)

		// Fix Heading
		headingMatches := headingRegex.FindStringSubmatch(lines[i])
		if len(headingMatches) > 0 {
			Info.Println("Found Heading: " + lines[i])
			newLine := ""
			for i := 0; i < len(headingMatches[1]); i++ {
				newLine = newLine + "#"
			}
			newLine = newLine + " " + headingMatches[2]
			lines[i] = newLine
		}

		// Fix Links
		lines[i] = linkRegex.ReplaceAllStringFunc(lines[i], func(s string) string {
			Info.Println("Found link " + s)

			s = strings.Trim(s, "[]")
			name := s
			link := s
			if strings.Contains(s, "|") {
				link = strings.TrimSpace(s[:strings.Index(s, "|")])
				name = strings.TrimSpace(s[strings.Index(s, "|")+1:])
			}

			Info.Printf("Link: \"%s\", Name: \"%s\"\n", link, name)

			if strings.Contains(link, "attachment:") {
				Info.Printf("Link to attachment, ignoring.")
				return "[[" + s + "]]"
			}

			link, successful := findLink(path, link)
			if !successful {
				Warning.Printf("Error finding link \"%s\"\n", link)
			}

			return "[" + name + "](" + link + ")"
		})

		// Fix inline code
		lines[i] = inlineCodeRegex.ReplaceAllString(lines[i], "`$1`")

		// Fix attachments
		lines[i] = attachmentRegex.ReplaceAllStringFunc(lines[i], func(s string) string {
			s = strings.Trim(s, "{[]}")
			s = strings.TrimPrefix(s, "attachment:")
			if strings.Contains(s, "|") {
				s = s[:strings.Index(s, "|")]
			}
			Info.Println("Found attachment: " + s)

			var isMedia = false
			for _, suffix := range mediaSuffixes {
				if hasSuffixIgnoreCase(s, suffix) {
					isMedia = true
					break
				}
			}

			if isMedia {
				Info.Println("Attachment is media.")
			}

			getAttachment(path, s)

			s = "[" + s + "](" + s + ")"
			if isMedia {
				s = "!" + s
			}
			Info.Println("Translated to " + s)
			return s
		})

		if strings.HasPrefix(strings.TrimSpace(lines[i]), "* ") {

			if currentBaseIndent == -1 {
				currentBaseIndent = strings.Index(lines[i], "*")
			}
			indent := 2 * (strings.Index(lines[i], "*") - currentBaseIndent)
			lines[i] = strings.TrimLeft(lines[i], "* ")
			lines[i] = "* " + lines[i]
			for b := 0; b < indent; b++ {
				lines[i] = " " + lines[i]
			}
		} else {
			currentBaseIndent = -1
		}

		lines[i] = boldRegex.ReplaceAllString(lines[i], "**$1**")
		lines[i] = italicRegex.ReplaceAllString(lines[i], "*$1*")
		lines[i] = strikethroughRegex.ReplaceAllString(lines[i], "~~$1~~")
	}

	_ = file.Close()

	newFile, err := os.OpenFile(path, os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		Error.Fatal(err)
	}

	for _, l := range lines {

		if strings.HasPrefix(l, "#DISCARD") {
			continue
		}

		// Achtung hack für tables
		if strings.Contains(l, "#TABLE-") {
			num := l[strings.Index(l, "#TABLE-")+7:]
			l = l[:strings.Index(l, "#TABLE-")]
			numCols, err := strconv.Atoi(num)
			if err != nil {
				Error.Fatal(err)
			}
			l += "\n"
			for i := 0; i < numCols; i++ {
				l += "|---"
			}

			l = "\n" + l + "|"
		}

		_, err := newFile.WriteString(l + "\n")
		if err != nil {
			Error.Fatal(err)
		}

	}
	_ = newFile.Close()

}

func findLink(path, link string) (string, bool) {
	Info.Printf("Searching link %s\n", link)
	currentDir := path[:strings.LastIndex(path, "/")]
	if link[0] == '/' {
		// Try Relative
		if testFileExists(currentDir+link) == DIR {
			return strings.TrimPrefix(currentDir+link+"/index", prefix+"/"), true
		} else if testFileExists(currentDir+link+".md") == FILE {
			return strings.TrimPrefix(currentDir+link, prefix+"/"), true
		}
	} else if strings.Contains(link, "://") {
		return link, true
	} else {
		if testFileExists(prefix+"/"+link) == DIR {
			return link + "/index", true
		} else if testFileExists(prefix+"/"+link+".md") == FILE {
			return link, true
		}
	}

	return link, false
}

type FileStatus int

const (
	NOTEXIST FileStatus = iota
	DIR
	FILE
)

func testFileExists(path string) FileStatus {
	stat, err := os.Stat(path)
	if os.IsNotExist(err) {
		return NOTEXIST
	} else if err != nil {
		return NOTEXIST
	}
	if stat.IsDir() {
		return DIR
	}
	return FILE
}

func hasSuffixIgnoreCase(s, suffix string) bool {
	return strings.HasSuffix(strings.ToLower(s), strings.ToLower(suffix))
}

func getAttachment(path, name string) {
	dir := path[:strings.LastIndex(path, "/")]
	path = strings.TrimSuffix(path, ".md")
	path = strings.TrimSuffix(path, "/index")
	name = strings.TrimSpace(name)

	Info.Println("Path is " + path)
	Info.Println("Name is " + name)
	attachmentUrl := wikiUrl + "/" + path + "?action=AttachFile&do=get&target=" + name
	Info.Println("Url is " + attachmentUrl)

	req, err := http.NewRequest("GET", attachmentUrl, nil)
	if err != nil {
		Error.Fatal(err)
	}
	req.AddCookie(&http.Cookie{
		Name:  sessionCookieName,
		Value: sessionCookie,
	})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		Error.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {

		r, _ := ioutil.ReadAll(resp.Body)
		Error.Println(string(r))
		Error.Println("Downloading attachment from " + attachmentUrl + " : HTTP status code != 200")
		time.Sleep(10 * time.Second)
		return
	}

	attachment, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		Error.Fatal(err)
	}
	Info.Println("got attachment of size " + strconv.Itoa(len(attachment)))
	Info.Println("storing attachment at " + dir + "/" + name)
	if testFileExists(dir+"/"+name) == FILE {
		Info.Println("already exists")
		return
	}
	err = ioutil.WriteFile(dir+"/"+name, attachment, 0644)
	if err != nil {
		Error.Fatal(err)
	}
	time.Sleep(13 * time.Second)
}
