package main

import (
	"bytes"
	"path/filepath"
	"os"
	"path"
	"http"
	"rand"
	"regexp"
	"template"
	"strings"
	"io/ioutil"
	"smtp"
	"time"
//	"fmt"
)

const (
	maxupload = 10e6
	largeupload = 2e6
	fileinform = "upload"
	taginform = "tag"
	idstring = "gogallery"
	newtag = "newtag"
	fullsize = "fullsize"
)

var (
	fileServer = http.FileServer(rootdir, "")
//TODO: since I had to accept % escapes for accented chars, it does not prevent spaces anymore. 
	tagValidator = regexp.MustCompile("^([a-zA-Z0-9]|_|-|%)+$")
	picValidator = regexp.MustCompile(".*(jpg|JPG|jpeg|JPEG|png|gif|GIF)$")
	templates = make(map[string]*template.Template)
	
)

var (
    errPassRequired = os.NewError("Password required for this operation")
	errLargeUpload = os.NewError("Upload too large")
)

type lines []string

func (p *lines) Write(line string) (n int, err os.Error) {
	slice := *p
    l := len(slice)
    if l == cap(slice) {  // reallocate
        // Allocate one more line
        newSlice := make([]string, l+1, l+1)
        // The copy function is predeclared and works for any slice type.
        copy(newSlice, slice)
        slice = newSlice
    }
	l++;
    slice = slice[0:l]
	slice[l-1] = line
    *p = slice
	return len(line), nil
}

type page struct {
	Title	string
	Protocol string
	Host	string
	Body	lines
}

func newPage(title string, body lines) *page {
	p := page{title, protocol, *host, body}
	return &p
}

func httpErr(err os.Error) {
	if err != nil {
		panic(err)
	}
}

func renderTemplate(w http.ResponseWriter, tmpl string, p *page) {
	err := templates[tmpl].Execute(w, p)
	httpErr(err)
}

func tagPage(tag string) *page {
	pics := getPics(tag)
	for i := 0; i<len(pics); i++ {
		dir, file := path.Split(pics[i])
		thumb := path.Join(dir, thumbsDir, file)
		pics[i] = "<a href=\"" + protocol + "://" + *host + 
			path.Join(picpattern, tag, pics[i]) +
			"\"><img src=\"" + protocol + "://" + *host + "/" + thumb + "\"/></a>"
	}
	return newPage(tag, pics)
}

func tagsPage() *page {
	title := "All tags"
	tags := getTags()
	return newPage(title, tags)
}

func tagHandler(w http.ResponseWriter, r *http.Request, urlpath string) {
	tag := urlpath[len(tagpattern):]
	if !tagValidator.MatchString(http.URLEscape(tag)) {
		http.NotFound(w, r)
		return
	}
	p := tagPage(tag)
	renderTemplate(w, tagName, p)
}

func picHandler(w http.ResponseWriter, r *http.Request, urlpath string) {
	if path.IsAbs(urlpath) {
		urlpath = urlpath[1:]
	}
	words := strings.Split(urlpath, filepath.SeparatorString, -1)
	pic := path.Join(words[2:]...)
//	pic := urlpath[len(picpattern):]
	err := r.ParseForm()
	httpErr(err)

	// get new tag from POST
	inputTag := ""
	inputPass := ""
	for k, v := range (*r).Form {
//		println(k)
//		for _,y := range (v) {
//			println(y)
//		}
		switch k {
		case newtag:
			// only allow single alphanumeric word tag 
			if tagValidator.MatchString(http.URLEscape(v[0])) {
				inputTag = v[0]
				if inputPass != "" {
					if !needPass || passOk(inputPass)  {
						m.Lock()
						insert(pic, inputTag)
						m.Unlock()
					} else {
						err = errPassRequired
						httpErr(err)
					}
				}
			}
		case "password":
			inputPass = v[0]
			if inputTag != "" {
				if !needPass || passOk(inputPass) {
					m.Lock()
					insert(pic, inputTag)
					m.Unlock()
				} else {
					err = errPassRequired
					httpErr(err)
				}
			} 
   		case fullsize:
			picPath := path.Join("/", pic)
			http.Redirect(w, r, picPath, http.StatusFound)
		}
	}
	
	dir, file := path.Split(pic)
	resized := path.Join(dir, resizedDir, file)
	if needResize(pic) {
			mkResized(pic)
	} else {
//TODO: mv that to a global check ran once at the beginning, so that we don't recheck it everytime?
		err := os.MkdirAll(path.Join(dir, resizedDir), 0755)
		httpErr(err)

		err = os.Symlink(path.Join("..",file), resized)
		if err != nil && err.(*os.LinkError).Error != os.EEXIST {
//TODO: let it fail silently?
			http.Error(w, err.String(), http.StatusInternalServerError)
		}
	}
	picSrc := lines{resized}
	p := newPage(path.Join(words[1], pic), picSrc)
	renderTemplate(w, picName, p)
}

func tagsHandler(w http.ResponseWriter, r *http.Request, urlpath string) {
	p := tagsPage()
	renderTemplate(w, tagsName, p)
}

func randomHandler(w http.ResponseWriter, r *http.Request, urlpath string) {
	randId := rand.Intn(maxId) + 1
	s := getNextId(randId)
	if s == "" {
		maxId = randId
		s = getPrevId(randId)
	}
	if s == "" {
		http.NotFound(w, r)
		return
	}
	s = path.Join(picpattern, allPics, s)
	http.Redirect(w, r, s, http.StatusFound)
}

//TODO: check that referer can never have a different *host part ?
func nextHandler(w http.ResponseWriter, r *http.Request, urlpath string) {
	ok, err := regexp.MatchString(
		"^" + protocol + "://"+*host+picpattern+".*$", (*r).Referer)
	httpErr(err)

//TODO: maybe print the 1st one instead of a 404 ?
	if !ok {		
		http.NotFound(w, r)
		return
	}
	prefix := len(protocol + "://" + *host)
	picPath := (*r).Referer[prefix:]
	if path.IsAbs(picPath) {
		picPath = picPath[1:]
	}
	words := strings.Split(picPath, filepath.SeparatorString, -1)
	file, err := http.URLUnescape(path.Join(words[2:]...))
	httpErr(err)

	tag, err := http.URLUnescape(words[1])
	httpErr(err)

	s := getNext(file, tag)
	if s == "" {
		s = file
	}
	s = path.Join(picpattern, tag, s)
	http.Redirect(w, r, s, http.StatusFound)
}

func prevHandler(w http.ResponseWriter, r *http.Request, urlpath string) {
	ok, err := regexp.MatchString(
		"^" + protocol + "://"+*host+picpattern+".*$", (*r).Referer)
	httpErr(err)

	if !ok {		
		http.NotFound(w, r)
		return
	}
	prefix := len(protocol + "://" + *host)
	picPath := (*r).Referer[prefix:]
	if path.IsAbs(picPath) {
		picPath = picPath[1:]
	}
	words := strings.Split(picPath, filepath.SeparatorString, -1)
	file, err := http.URLUnescape(path.Join(words[2:]...))
	httpErr(err)

	tag, err := http.URLUnescape(words[1])
	httpErr(err)

	s := getPrev(file, tag)
	if s == "" {
		s = file
	}
	s = path.Join(picpattern, tag, s)
	http.Redirect(w, r, s, http.StatusFound)
}

func uploadHandler(w http.ResponseWriter, r *http.Request, urlpath string) {
	p := newPage("", nil)
	tag := ""
	filepath := ""

	reader, err := r.MultipartReader()

	// do nothing if no form
//TODO: should I not look for the uploadpattern action string?
	if err == nil {
		for {
			part, err := reader.NextPart()
			httpErr(err)

			if part == nil {
				break
			}
			partName := part.FormName()
			// get the file
			if partName == fileinform {
				// get the filename 
//TODO: use new native func to do that
				line := part.Header.Get("Content-Disposition")
				filename := line[strings.Index(line, "filename="):]
//TODO: that will fail if it's not at the end of the line, do better with a regex
				filename = filename[10:len(filename)-1]
				// get the upload
//TODO: sizing the buffer and then checking n indeed prevents filling mem and/or disk, but the thing will still be uploaded fully -> waste of b/w. => do better?
				b := bytes.NewBuffer(make([]byte, 0, maxupload))
				n, err := b.ReadFrom(part)
				if err != nil {
					if err != os.EOF {
						http.Error(w, err.String(), http.StatusInternalServerError)
						return
					}
				}
				if n > maxupload {
					err = errLargeUpload
					panic(err)
				}
				if n > largeupload && *conffile != "" {
					err = smtp.SendMail(config.Email.Server, nil, config.Email.From, config.Email.To, []byte(config.Email.Message))
					httpErr(err)
				}
				// write file in dir with YYYY-MM-DD format
				filedir := path.Join(config.Picsdir, time.UTC().Format("2006-01-02"))
				err = mkdir(filedir)
				httpErr(err)
				// create thumbsdir while we're at it
				err = mkdir(path.Join(filedir, thumbsDir))
				httpErr(err)
				// finally write the file
				filepath = path.Join(filedir, filename)
				err = ioutil.WriteFile(filepath, b.Bytes(), 0644)
//				err = ioutil.WriteFile(filepath, upload, 0644)
				httpErr(err)
				p.Title = filename + ": upload sucessfull"
				if tag != "" {
					// tag is set, hence it has already been found 
					break
				}
				continue
			}
			// get the tag
			if partName == taginform {
				b := make([]byte, 128)
				n, err := part.Read(b)
//TODO: better err handling ?
				if err == nil {
					b = b[0:n]
					tag = string(b)
				}
				if p.Title != "" {
					// Title is set, hence upload has already been done
					break;
				}
			}
		}
		// only insert tag if we have an upload of a pic and a tag for it			
		if tag != "" && p.Title != "" {
			if tagValidator.MatchString(http.URLEscape(tag)) && 
				picValidator.MatchString(filepath) {
				err = mkThumb(filepath)
				httpErr(err)
				m.Lock()
				insert(filepath[rootdirlen+1:], tag)
				m.Unlock()
			}
		}
	}
	renderTemplate(w, upName, p)
}

func serveFile(w http.ResponseWriter, r *http.Request) {
	fileServer.ServeHTTP(w, r);
}

func makeHandler(fn func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if e, ok := recover().(os.Error); ok {
				http.Error(w, e.String(), http.StatusInternalServerError)
				return
			}
		}()
		title := r.URL.Path
		w.Header().Set("Server", idstring)
		fn(w, r, title)
	}
}
