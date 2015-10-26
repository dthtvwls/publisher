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

var src, dest, port, user, pass, bucket, access_key, secret_key string

func init() {
	flag.StringVar(&src, "src", "", "(Required) URL of the site to pull from")
	flag.StringVar(&dest, "dest", "", "(Required) Directory to keep snapshots in")
	flag.StringVar(&port, "port", "", "(Required) Port for the web interface to listen on")
	flag.StringVar(&user, "user", "", "(Optional) HTTP auth username")
	flag.StringVar(&pass, "pass", "", "(Optional) HTTP auth password")
	flag.StringVar(&bucket, "bucket", "", "(Required) S3 bucket to sync to after publishing locally (i.e. symlinking)")
	flag.StringVar(&access_key, "access_key", "", "(Required) AWS key for S3")
	flag.StringVar(&secret_key, "secret_key", "", "(Required) AWS secret for S3")
}

func execute(w http.ResponseWriter, args ...string) error {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = w
	cmd.Stderr = w
	return cmd.Run()
}

func main() {
	flag.Parse()

	if src == "" || dest == "" || port == "" || bucket == "" || access_key == "" || secret_key == "" {
		log.Fatal("Some required arguments are missing (see -h for help)")
	}

	uri, err := url.Parse(src)
	if err != nil {
		panic(err)
	}
	if !uri.IsAbs() {
		log.Fatal("Source must be an absolute URL")
	}
	src = uri.String()

	if !filepath.IsAbs(dest) {
		log.Fatal("Please use absolute paths")
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			// put the snapshot in its own, timestamped directory (in ISO 8601 because standards)
			dir := filepath.Join(dest, time.Now().UTC().Format("2006-01-02T15:04:05Z"))

			if err := os.MkdirAll(dir, 0755); err != nil {
				panic(err)
			}

			// TODO: figure out chunked data because a curl call (for example) might bail before the wget job is done
			if err := execute(w, "wget", "--mirror", "--page-requisites", "--adjust-extension", "--convert-links",
				"--no-host-directories", "--http-user="+user, "--http-password="+pass, "--directory-prefix="+dir, src); err != nil {
				// wget will have almost certainly tried some requests that returned http errors, and thus return 8:
				//   http://www.gnu.org/software/wget/manual/html_node/Exit-Status.html
				// so if the error was an "exit error", and the status is 8, let's not panic
				if exiterr, ok := err.(*exec.ExitError); !ok || exiterr.Sys().(syscall.WaitStatus).ExitStatus() != 8 {
					panic(err)
				}
			}

			if err := os.Setenv("ACCESS_KEY", access_key); err != nil {
				panic(err)
			}
			if err := os.Setenv("SECRET_KEY", secret_key); err != nil {
				panic(err)
			}

			if err := execute(w, "s3cmd", "sync", dir+"/", "--delete-removed", "s3://"+bucket); err != nil {
				panic(err)
			}
			if err := execute(w, "s3cmd", "modify", "s3://"+bucket+"/**/*.css", "--mime-type='text/css'"); err != nil {
				panic(err)
			}
			if err := execute(w, "s3cmd", "modify", "s3://"+bucket+"/*.html", "--mime-type='text/html; charset=utf-8'"); err != nil {
				panic(err)
			}

		} else {
			http.ServeFile(w, r, "publisher.html")
		}
	})

	http.ListenAndServe(":"+port, nil)
}
