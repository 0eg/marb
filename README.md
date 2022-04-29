# marb

marb is a highly opinionated HTTP server for static sites. To give you
a hint of just how opinionated it is: marb will load all the files in
memory and that's it. There's no hot-reloading: you need to restart it
on content update.

All files are gzipped, except for cases when the gzipped version results
in a bigger file size. Rudimentary caching is supported via the
`Last-Modified` and `If-Modified-Since` headers.

## Usage

Here I'll refer to `marb`, which is the output of running `go build`.
Generally you can replace `marb` with `go run main.go` and get the same
results.

Here's how you can run marb, using all of its options:

```
marb -root /path/to/site -404 error.html -index main.html -bind host:1234
```

Below you'll see the list of options and their defaults, which you can
get any time by running `marb -h`:

```
  -404 string
        fallback file on error 404, relative to the root
  -bind string
        the address to bind to (default "0.0.0.0:7890")
  -index string
        index file name (default "index.html")
  -root string
        the root directory to serve files from (default "/var/www/")
```

## Using with Docker

The Dockerfile in this repo is the one used to build the image, which
lives in https://hub.docker.com/r/00eg/marb.

If you want to run marb in a Docker instance, below you'll find an example
Dockerfile:

```Dockerfile
FROM 00eg/marb
COPY ./public/ /var/www/
EXPOSE 7890
ENTRYPOINT ["/bin/marb", "-404", "404.html"]
```

The above Dockerfile will copy files in `./public` to the `/var/www/`
directory, which is the default root directory. It exposes port 7890,
which is the port marb listens on by default, and finally runs
`/bin/marb -404 404.html` which specifies a file to serve on error 404.