package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/urso/ecslog"
	"github.com/urso/ecslog/backend"
	"github.com/urso/ecslog/backend/enclog"
	"github.com/urso/ecslog/backend/jsonlog"
	"github.com/urso/ecslog/backend/objlog"
	"github.com/urso/ecslog/backend/structlog"
	"github.com/urso/ecslog/backend/txtlog"
	"github.com/urso/ecslog/errx"
	"github.com/urso/ecslog/fld"
	"github.com/urso/ecslog/fld/ecs"
)

func main() {
	mode := "text"
	flag.StringVar(&mode, "mode", "text", "select print mode")
	flag.Parse()

	modes := map[string]func(){
		"text": func() {
			testWith("text message only", ecslog.New(
				txtlog.NewTextBackend(txtlog.Writer(os.Stdout, backend.Trace, false))))
		},
		"verbose": func() {
			testWith("text message only", ecslog.New(
				txtlog.NewTextBackend(txtlog.Writer(os.Stdout, backend.Trace, true))))
		},
		"json": func() {
			json, err := jsonlog.New(enclog.Writer(os.Stdout, backend.Trace, "\n"), []fld.Field{
				structlog.DynTimestamp(time.RFC3339Nano),
			})
			if err != nil {
				panic(err)
			}
			testWith("", ecslog.New(json))
		},
		"obj": func() {
			backend, err := objlog.New(
				objlog.Call(backend.Trace, func(obj map[string]interface{}) {
					spew.Dump(obj)
				}),
				[]fld.Field{
					structlog.DynTimestamp(time.RFC3339Nano),
				},
			)
			if err != nil {
				panic(err)
			}

			testWith("obj", ecslog.New(backend))
		},
	}

	fn, ok := modes[mode]
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown mode: %v\n", mode)
		os.Exit(1)
	}

	fn()
}

func testWith(title string, log *ecslog.Logger) {
	if title != "" {
		printTitle(title)
	}
	defer fmt.Println()

	log.Trace("trace message")
	log.Debug("debug message")
	log.Info("info message")
	log.Error("error message")

	log.Info("with std format string: %v", "test")
	log.Info("with std format string '%v' and more", "test")

	log.Info("info with %{custom} message", "user")
	log.Info("info with %{custom} message and number of %{number}", "user", 42)
	log.With(
		"bool", true,
		"int", 42,
		"op", "some-op",
	).Info("info with extra fields, %{custom} message, and %{number}", "user", 42)

	log.With(
		ecs.Host.Hostname("localhost"),
	).Info("info with ecs field, %{custom} message, and %{number}", "user", 42)

	log.With("field", 1).Info("logger overwriting 'field' with %{field}", 2)

	log.Info("set 'field' %{field} and change 'field' to %{field}", 1, 2)

	log.Error("log error value: %{reason}", errors.New("oops"))

	log.Error("log errx formatted: %{reason}",
		errx.Errf("ooops with %{extra}", "value"))

	log.Error("log errx formatted with user field: %{reason}",
		errx.With("field", 1).Errf("ooops with %{extra}", "value"))

	log.Error("log errx formatted with ecs field: %{reason}",
		errx.With(ecs.Host.Hostname("localhost")).Errf("ooops with %{extra}", "value"))

	log.Error("log errx verbose formatted: %{+reason}",
		errx.Errf("ooops with %{extra}", "value"))

	log.Error("wrap EOF error with location: %v",
		errx.Wrap(io.EOF, "failed to read %{file}", "file.txt"))

	log.Error("wrap EOF twice: %v",
		errx.Wrap(
			errx.Wrap(io.EOF, "unepxected end of file in %{file}", "file.txt"),
			"error reading files in %{dir}", "path/to/files"))

	log.Error("wrap EOF with Errf: %v",
		errx.Errf("failed to read %{file}", "file.txt", io.EOF))

	log.With(
		ecs.Service.Name("my server"),
		ecs.Host.Hostname("localhost"),
		"custom", "value",
		"nested.custom", "another value",
	).With(
		ecs.HTTP.Request.Method("GET"),
		ecs.URL.Path("/get_file/file.txt"),
		ecs.Source.Domain("localhost"),
		ecs.Source.IP("127.0.0.1"),
	).Error("wrap unexpected EOF with additional fields: %v",
		errx.With(
			ecs.File.Path("file.txt"),
			ecs.File.Extension("txt"),
			ecs.File.Owner("me"),
		).Wrap(io.EOF, "failed to read file"))

	log.With(
		"test", "field",
	).Error("Can not open keystore: %{error}",
		errx.With("op", "db/open").Wrap(
			errx.With("op", "db/init").Wrap(io.EOF,
				"failed to read db header in %{file}", "dbname/file.db"),
			"can not open database %{database}", "dbname"))

	log.Error("many errors: %v",
		errx.WrapAll([]error{
			io.EOF,
			io.ErrClosedPipe,
		}, "init operation failed"))

	log.Error("wrapped many errors tree: %v",
		errx.WrapAll([]error{
			errx.Wrap(io.EOF, "unexpected eof in %{file}", "tx.log"),
			errx.Wrap(io.ErrClosedPipe, "remote connection to %{server} closed", "localhost"),
		}, "init operation failed"))

	log.Error("multiple errors: %v || %v",
		errx.Wrap(io.EOF, "unexpected eof in %{file}", "tx.log"),
		errx.Wrap(io.ErrClosedPipe, "remote connection to %{server} closed", "localhost"),
	)
}

func printTitle(title string) {
	fmt.Println(title)
	for range title {
		fmt.Print("-")
	}
	fmt.Println()
}
