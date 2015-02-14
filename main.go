package main

import (
	"exec"
	"flag"
	"fmt"
	"http"
	"io/ioutil"
	"json"
	"log"
	"os"
	"path"
	"regexp"
	"crypto/sha1"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"template"
	sqlite "gosqlite.googlecode.com/hg/sqlite"
)

//TODO: enable/disable public tags? auth?
//TODO: clean command to remove .thumbs or .resized
//TODO: add last selected tag (and all tags) as a link?
//TODO: cloud of tags? variable font size?
//TODO: session? with cookies?
//TODO: preload all the picsPaths for a given tag?
//TODO: be nicer to file paths with spaces?

const (
	thumbsDir = ".thumbs"
	resizedDir = ".resized"
	picpattern = "/pic/"
	tagpattern = "/tag/"
	tagspattern = "/tags"
// security through obscurity for now; just don't advertize your uploadpattern if you don't want others to upload to your server
	uploadpattern = "/upload"
	allPics = "all"
)

var (
	rootdir, _ = os.Getwd()
	rootdirlen = len(rootdir)
	config conf = conf{
		Dbfile: "./gallery.db",
		Initdb: false,
		Picsdir: "./",
		Thumbsize: "200x300",
		Normalsize: "800x600",
		Tmpldir: "",
		Norand: false,
		Password: "",
		Tls: false}
	maxWidth int
	maxHeight int
	m *sync.Mutex = new(sync.Mutex)
	convertBin string
	identifyBin string
	needPass bool
	protocol string = "http"
)

var (
	conffile = flag.String("conf", "", "json conf file to send email alerts")
	host       = flag.String("host", "localhost:8080", "listening port and hostname that will appear in the urls")
	help       = flag.Bool("h", false, "show this help")
)

type conf struct {
	Email emailConf
	Dbfile string
	Initdb bool
	Picsdir string
	Thumbsize string
	Normalsize string
	Tmpldir string
	Norand bool
//this password is supposed to be a sha1sum
	Password string
	Tls bool
}

type emailConf struct {
	Server string
	From string
	To []string
	Message string
}

func readConf(confFile string) os.Error {
	r, err := os.Open(confFile)
	if err != nil {
		log.Fatal(err)
	}
	dec := json.NewDecoder(r)
	err = dec.Decode(&config)
	if err != nil {
		log.Fatal(err)
	}
	r.Close()
	sizes := strings.Split(config.Normalsize, "x", -1)
	if len(sizes) != 2 { 
		return os.NewError("Invalid Normalsize value \n")
	}
	maxWidth, err = strconv.Atoi(sizes[0])
	errchk(err)
	maxHeight, err = strconv.Atoi(sizes[1])
	errchk(err)
	needPass = false
	if config.Password != "" {
		needPass = true
	}
	if config.Tls {
		protocol = "https"
	}
	return nil
}

func passOk(pass string) bool {
	sha := sha1.New()
	sha.Write([]byte(pass))
	return config.Password == fmt.Sprintf("%x", string(sha.Sum()))
}

func mkdir(dirpath string) os.Error {
	// used syscall because can't figure out how to check EEXIST with os
	e := 0
	e = syscall.Mkdir(dirpath, 0755)
	if e != 0 && e != syscall.EEXIST {
		return os.Errno(e)
	}
	return nil
}

func scanDir(dirpath string, tag string) os.Error {
	if strings.Contains(dirpath, " ") {
		log.Print("Skipping " + dirpath + " because spaces suck\n")
		return nil
	}
	currentDir, err := os.Open(dirpath)
	if err != nil {
		return err
	}
	names, err := currentDir.Readdirnames(-1)
	if err != nil {
		return err
	}
	currentDir.Close()
	sort.SortStrings(names)
	err = mkdir(path.Join(dirpath, thumbsDir))
	if err != nil {
		return err
	}
	for _, v := range names {
		childpath := path.Join(dirpath, v)
		if strings.Contains(childpath, " ") {
			log.Print("Skipping " + childpath + " because spaces suck\n")
			continue
		}
		fi, err := os.Lstat(childpath)
		if err != nil {
			return err
		}
		if fi.IsDirectory() && v != thumbsDir && v != resizedDir {
			err = scanDir(childpath, tag)
			if err != nil {
				return err
			}
		} else {
			if picValidator.MatchString(childpath) {
				err = mkThumb(childpath)
				if err != nil {
					return err
				}
				path := childpath[rootdirlen+1:]
				m.Lock()
				insert(path, tag)
				m.Unlock()
			}
		}

	}
	return err
}

func getBinsPaths() {
	var err os.Error
	convertBin, err = exec.LookPath("convert")
	if err != nil {
		newErr := os.NewError(err.String() + "\n it usually comes with imagemagick")
		log.Fatal(newErr)
	}
	identifyBin, err = exec.LookPath("identify")
	if err != nil {
		newErr := os.NewError(err.String() + "\n it usually comes with imagemagick")
		log.Fatal(newErr)
	}
}

//TODO: set up a pool of goroutines to do the converts concurrently (probably not a win on a monocore though)
func mkThumb(filepath string) os.Error {
	dir, file := path.Split(filepath)
	thumb := path.Join(dir, thumbsDir, file)
	_, err := os.Stat(thumb)
	if err == nil {
		return nil
	}
	args := []string{convertBin, filepath, "-thumbnail", config.Thumbsize, thumb}
	fds := []*os.File{os.Stdin, os.Stdout, os.Stderr}
	p, err := os.StartProcess(args[0], args, &os.ProcAttr{Files: fds})
	if err != nil {
		return err
	}
	_, err = os.Wait(p.Pid, os.WSTOPPED)
	if err != nil {
		return err
	}
	return nil
}

//TODO: mv to an image.go file
//TODO: use native go stuff to do the job when it's ready. as of 05/2011, at least decoding jpeg is pretty unreliable.
func needResize(pic string) bool {
	pr, pw, err := os.Pipe()
	if err != nil {
		log.Fatal(err)
	}
	args := []string{identifyBin, pic}
	fds := []*os.File{os.Stdin, pw, os.Stderr}
	p, err := os.StartProcess(args[0], args, &os.ProcAttr{Files: fds})
	if err != nil {
		log.Fatal(err)
	}
	pw.Close()
	_, err = os.Wait(p.Pid, os.WSTOPPED)
	if err != nil {
		log.Fatal(err)
	}
	buf, err := ioutil.ReadAll(pr)
	if err != nil {
		log.Fatal(err)
	}
	pr.Close()
	output := strings.Split(string(buf), " ", -1)
	// resolution should be the 3rd one of the output of identify
	res := strings.Split(output[2], "x", -1)
	w, err := strconv.Atoi(res[0])
	if err != nil {
		log.Fatal(err)
	}
	h, err := strconv.Atoi(res[1])
	if err != nil {
		log.Fatal(err)
	}
	return w > maxWidth || h > maxHeight
}

// we can use convert -resize/-scale because, like -thumbnail, they conserve proportions 
func mkResized(pic string) os.Error {
	dir, file := path.Split(pic)
	resized := path.Join(dir, resizedDir, file)
	_, err := os.Stat(resized)
	if err == nil {
		return nil
	}
	err = os.MkdirAll(path.Join(dir, resizedDir), 0755)
	if err != nil {
		return err
	}	
	args := []string{convertBin, pic, "-resize", config.Normalsize, resized}
	fds := []*os.File{os.Stdin, os.Stdout, os.Stderr}
	p, err := os.StartProcess(args[0], args, &os.ProcAttr{Files: fds})
	if err != nil {
		return err
	}
	_, err = os.Wait(p.Pid, os.WSTOPPED)
	if err != nil {
		return err
	}
	return nil
}

func chkpicsdir() {
	// fullpath for picsdir. must be within document root
	config.Picsdir = path.Clean(config.Picsdir)
	if (config.Picsdir)[0] != '/' {
		cwd, _ := os.Getwd()
		config.Picsdir = path.Join(cwd, config.Picsdir)
	}
	pathValidator := regexp.MustCompile(rootdir + ".*")
	if !pathValidator.MatchString(config.Picsdir) {
		log.Fatal("picsdir has to be a subdir of rootdir. (symlink ok)")
	}
}

func chktmpl() {
	if config.Tmpldir == "" {
		config.Tmpldir = basicTemplates
		err := mkTemplates(config.Tmpldir)
		if err != nil {
			log.Fatal(err)
		}
	}
	// same drill for templates.
	config.Tmpldir = path.Clean(config.Tmpldir)
	if (config.Tmpldir)[0] != '/' {
		cwd, _ := os.Getwd()
		config.Tmpldir = path.Join(cwd, config.Tmpldir)
	}
	pathValidator := regexp.MustCompile(rootdir + ".*")
	if !pathValidator.MatchString(config.Tmpldir) {
		log.Fatal("tmpldir has to be a subdir of rootdir. (symlink ok)")
	}
	for _, tmpl := range []string{tagName, picName, tagsName, upName} {
		templates[tmpl] = template.MustParseFile(path.Join(config.Tmpldir, tmpl+".html"), nil)
	}
}

//TODO: add some other potential risky chars
func badchar(filepath string) (bool, string) {
	len0 := len(filepath)
	n := strings.IndexRune(filepath, '\'')
	for n != -1 {
		filepath = filepath[0:n] + filepath[n+1:]
		n = strings.IndexRune(filepath, '\'')
	}
	if len(filepath) != len0 {
		return true, filepath
	}
	return false, ""
}

func errchk(err os.Error) {
	if err != nil {
		log.Fatal(err)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, "usage: \n\t gogallery tag dir tagname\n")
	fmt.Fprintf(os.Stderr, "\t gogallery deltag tagname \n")
	fmt.Fprintf(os.Stderr, "\t gogallery \n")
	flag.PrintDefaults()
	os.Exit(2)
}

func main() {
	flag.Usage = usage
	flag.Parse()
	if *help {
		usage()
	}

	getBinsPaths()
	if *conffile != "" {
		errchk(readConf(*conffile))
	}
	chkpicsdir()

	// tag or del cmds 
	nargs := flag.NArg()
	if nargs > 0 {
		if nargs < 2 {
			usage()
		}
		var err os.Error
		cmd := flag.Args()[0]
		switch cmd {
		case "tag":
			if nargs < 3 {
				usage()
			}
//why the smeg can't I use := here? 
			db, err = sqlite.Open(config.Dbfile)
			errchk(err)
			config.Picsdir = flag.Args()[1]
			chkpicsdir()
			tag := flag.Args()[2]
			errchk(scanDir(config.Picsdir, tag))
			log.Print("Scanning of " + config.Picsdir + " complete.")
			db.Close()
		case "deltag":
			db, err = sqlite.Open(config.Dbfile)
			errchk(err)
			delete(flag.Args()[1])
			db.Close()
		default:
			usage()		
		}
		return
	}

	// web server mode
	chktmpl()
	if config.Initdb {
		initDb()
	} else {
		var err os.Error
		db, err = sqlite.Open(config.Dbfile)
		errchk(err)
	}
	setMaxId()

	http.HandleFunc(tagpattern, makeHandler(tagHandler))
	http.HandleFunc(picpattern, makeHandler(picHandler))
	http.HandleFunc(tagspattern, makeHandler(tagsHandler))
	http.HandleFunc("/random", makeHandler(randomHandler))
	http.HandleFunc("/next", makeHandler(nextHandler))
	http.HandleFunc("/prev", makeHandler(prevHandler))
	http.HandleFunc(uploadpattern, makeHandler(uploadHandler))
	http.HandleFunc("/", http.HandlerFunc(serveFile))
	if config.Tls {
		http.ListenAndServeTLS(*host, "cert.pem", "key.pem", nil)
	} else {
		http.ListenAndServe(*host, nil)
	}
}
