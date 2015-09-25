package main

import (
	"flag"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

var src, dest, link, port string

func init() {
	flag.StringVar(&src, "src", "", "URL of the site to pull from")
	flag.StringVar(&dest, "dest", "", "Directory to keep snapshots in")
	flag.StringVar(&link, "link", "", "Location to symlink the latest snapshot to (e.g., for a static file server to serve)")
	flag.StringVar(&port, "port", "", "Port for the web interface to listen on")
}

func main() {
	flag.Parse()

	if src == "" || dest == "" || link == "" || port == "" {
		log.Fatal("All arguments are required (see -h for help)")
	}

	uri, err := url.Parse(src)
	if err != nil {
		panic(err)
	}
	if !uri.IsAbs() {
		log.Fatal("Source must be an absolute URL")
	}
	src = uri.String()

	if !filepath.IsAbs(dest) || !filepath.IsAbs(link) {
		log.Fatal("Please use absolute paths")
	}

	http.HandleFunc("/", handler)
	http.ListenAndServe(":"+port, nil)
}

func handler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		// put the snapshot in its own, timestamped directory (in ISO 8601 because standards)
		dir := filepath.Join(dest, time.Now().UTC().Format("2006-01-02T15:04:05Z"))

		if err := os.MkdirAll(dir, 0755); err != nil {
			panic(err)
		}

		cmd := exec.Command("wget", "--mirror", "--page-requisites", "--adjust-extension", "--convert-links", "--no-host-directories", "--directory-prefix="+dir, src)
		cmd.Stdout = w
		cmd.Stderr = w

		if err := cmd.Run(); err != nil {
			// wget will have almost certainly tried some requests that returned http errors, and thus return 8:
			//   http://www.gnu.org/software/wget/manual/html_node/Exit-Status.html
			// so if the error was an "exit error", and the status is 8, let's not panic
			if exiterr, ok := err.(*exec.ExitError); !ok || exiterr.Sys().(syscall.WaitStatus).ExitStatus() != 8 {
				panic(err)
			}
		}

		// stopping the program flow if there's a problem removing the old link is undesirable (what if it didn't exist?)
        // if there was some other kind of problem it will surface in the next call when we try to symlink
		os.Remove(link)

		if err := os.Symlink(dir, link); err != nil {
			panic(err)
		}
	} else {
		http.ServeFile(w, r, "publisher.html")
	}
}
