package main

import (
	"bytes"
	"compress/gzip"
	"flag"
	"fmt"
	"log"
	"mime"
	"net/http"
	"os"
	"path"
	"strings"
	"time"
)

type siteFile struct {
	contents     []byte
	mimeType     string
	isGzipped    bool
	name         string
	dir          string
	lastModified time.Time
}

func (f *siteFile) SetHeaders(h http.Header) {
	h.Set("Content-Length", fmt.Sprint(len(f.contents)))
	h.Set("Content-Type", f.mimeType)
	h.Set("Last-Modified", f.lastModified.Format(http.TimeFormat))
	if f.isGzipped {
		h.Set("Content-Encoding", "gzip")
	}
}

func compressContents(contents []byte) ([]byte, bool) {
	var buf bytes.Buffer

	zw := gzip.NewWriter(&buf)
	_, err := zw.Write(contents)
	zw.Close()

	if err != nil {
		log.Printf("could not gzip: %v", err)
		return nil, false
	}

	if buf.Len() >= len(contents) {
		// if size doesn't get reduced, then what's the point?
		return nil, false
	}

	return buf.Bytes(), true
}

func readFile(name string, size int) (*siteFile, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	file := &siteFile{
		name:      path.Base(name),
		dir:       path.Dir(name),
		contents:  make([]byte, size),
		isGzipped: false,
		mimeType:  mime.TypeByExtension(path.Ext(name)),
	}

	if _, err = f.Read(file.contents); err != nil {
		return nil, err
	}

	if file.mimeType == "" {
		file.mimeType = http.DetectContentType(file.contents)
	}

	gzipped, ok := compressContents(file.contents)
	if ok {
		file.contents = gzipped
		file.isGzipped = true
	}

	return file, nil
}

type memoryFileServer struct {
	name         string
	root         string
	files        map[string]*siteFile
	index        string
	error404     *siteFile
	error404Name string
	forceHTTPS   bool
	addrHeader   string
}

func (s *memoryFileServer) loadFiles(curPath string) error {
	fi, err := os.Lstat(curPath)
	if err != nil {
		return err
	}

	if fi.Mode()&os.ModeSymlink == os.ModeSymlink {
		if fi.IsDir() {
			return fmt.Errorf("%s: directory symbolic links are not allowed", curPath)
		}
		fi, err = os.Stat(curPath)
		if err != nil {
			return err
		}
	}

	if !fi.IsDir() {
		f, err := readFile(curPath, int(fi.Size()))

		if err != nil {
			return err
		}

		f.lastModified = fi.ModTime()
		s.addFile(f)

		return nil
	}

	f, err := os.ReadDir(curPath)
	if err != nil {
		return err
	}

	for _, p := range f {
		if err = s.loadFiles(path.Join(curPath, p.Name())); err != nil {
			return err
		}
	}

	return nil
}

func (s *memoryFileServer) addFile(f *siteFile) {
	if f.name == s.index {
		s.files[f.dir] = f
	}
	s.files[path.Join(f.dir, f.name)] = f
}

func (s *memoryFileServer) resolveFile(p string) *siteFile {
	return s.files[path.Join(s.root, p)]
}

func (s *memoryFileServer) serveOptions(w http.ResponseWriter) {
	allowedMethods := []string{http.MethodOptions, http.MethodGet, http.MethodHead}
	w.Header().Set("Allow", strings.Join(allowedMethods, ", "))
	w.WriteHeader(http.StatusNoContent)
}

func (s *memoryFileServer) serve404(w http.ResponseWriter, r *http.Request) {
	if s.error404 == nil {
		http.NotFound(w, r)
		return
	}

	s.error404.SetHeaders(w.Header())
	w.WriteHeader(http.StatusNotFound)

	if r.Method != http.MethodHead {
		w.Write(s.error404.contents)
	}
}

func (s *memoryFileServer) redirectIndex(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, path.Dir(r.URL.Path), http.StatusMovedPermanently)
}

func (s *memoryFileServer) redirectToHTTPS(w http.ResponseWriter, r *http.Request) {
	host := s.name
	if host == "" {
		host = r.Host
	}
	http.Redirect(w, r, "https://"+host+r.RequestURI, http.StatusMovedPermanently)
}

func (s *memoryFileServer) shouldRedirectToHTTPS(r *http.Request) bool {
	if !s.forceHTTPS {
		return false
	}

	return r.Header.Get("X-Forwarded-Proto") == "http"
}

func (s *memoryFileServer) serveFile(w http.ResponseWriter, r *http.Request) {
	f := s.resolveFile(r.URL.Path)

	if f == nil {
		s.serve404(w, r)
		return
	}

	if path.Base(r.URL.Path) == s.index {
		s.redirectIndex(w, r)
		return
	}

	if modSince := r.Header.Get("If-Modified-Since"); modSince != "" {
		modSinceTime, err := time.Parse(http.TimeFormat, modSince)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if !modSinceTime.Before(f.lastModified) {
			f.SetHeaders(w.Header())
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}

	f.SetHeaders(w.Header())

	if r.Method != http.MethodHead {
		w.Write(f.contents)
	}
}

func (s *memoryFileServer) logRequest(r *http.Request) {
	var clientAddr string
	if s.addrHeader != "" {
		clientAddr = r.Header.Get(s.addrHeader)
	} else {
		clientAddr = r.RemoteAddr
	}

	log.Printf("%s %s %s", clientAddr, r.Method, r.RequestURI)
}

func (s *memoryFileServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.logRequest(r)

	if s.shouldRedirectToHTTPS(r) {
		s.redirectToHTTPS(w, r)
		return
	}

	switch r.Method {
	case http.MethodOptions:
		s.serveOptions(w)
	case http.MethodGet, http.MethodHead:
		s.serveFile(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func newFileServer(name string, root string, index string, fourOhFour string, forceHTTPS bool, addrHeader string) (*memoryFileServer, error) {
	s := &memoryFileServer{
		name:         name,
		root:         root,
		files:        make(map[string]*siteFile),
		index:        index,
		error404Name: fourOhFour,
		forceHTTPS:   forceHTTPS,
		addrHeader:   addrHeader,
	}
	if err := s.loadFiles(root); err != nil {
		return nil, err
	}
	s.error404 = s.files[path.Join(s.root, fourOhFour)]
	return s, nil
}

var (
	bindAddr   = flag.String("bind", "0.0.0.0:7890", "the address to bind to")
	rootDir    = flag.String("root", "/var/www/", "the root directory to serve files from")
	notFound   = flag.String("404", "", "fallback file on error 404, relative to the root")
	indexFile  = flag.String("index", "index.html", "index file name")
	forceHTTPS = flag.Bool("https", false, "force HTTPS, based on X-Forwarded-Proto header")
	serverName = flag.String("name", "", "server name, used for HTTPS redirects (e.g example.com)")
	addrHeader = flag.String("addrHeader", "", "HTTP header which contains the client address")
)

func main() {
	flag.Parse()

	srv, err := newFileServer(*serverName, *rootDir, *indexFile, *notFound, *forceHTTPS, *addrHeader)
	if err != nil {
		log.Fatal(err)
	}

	log.Fatal(http.ListenAndServe(*bindAddr, srv))
}
