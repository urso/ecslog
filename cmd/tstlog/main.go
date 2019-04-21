package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/urso/ecslog"
	"github.com/urso/ecslog/backend"
	"github.com/urso/ecslog/backend/appender"
	"github.com/urso/ecslog/backend/layout"
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
			testWith(appender.Console(backend.Trace, layout.Text(false)))
		},
		"verbose": func() {
			testWith(appender.Console(backend.Trace, layout.Text(true)))
		},
		"json": func() {
			testWith(appender.Console(
				backend.Trace,
				layout.JSON([]fld.Field{
					layout.DynTimestamp(time.RFC3339Nano),
				}),
			))
		},
	}

	fn, ok := modes[mode]
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown mode: %v\n", mode)
		os.Exit(1)
	}

	fn()
}

func testWith(backend backend.Backend, err error) {
	defer fmt.Println()

	if err != nil {
		panic(err)
	}

	log := ecslog.New(backend)

	log.Trace("trace message")
	log.Debug("debug message")
	log.Info("info message")
	log.Error("error message")

	log.Infof("with std format string: %v", "test")
	log.Infof("with std format string '%v' and more", "test")

	log.Infof("info with %{custom} message", "user")
	log.Infof("info with %{custom} message and number of %{number}", "user", 42)
	log.With(
		"bool", true,
		"int", 42,
		"op", "some-op",
	).Infof("info with extra fields, %{custom} message, and %{number}", "user", 42)

	log.With(
		ecs.Host.Hostname("localhost"),
	).Infof("info with ecs field, %{custom} message, and %{number}", "user", 42)

	log.With("field", 1).Infof("logger overwriting 'field' with %{field}", 2)

	log.Infof("set 'field' %{field} and change 'field' to %{field}", 1, 2)

	log.Errorf("log error value: %{reason}", errors.New("oops"))

	log.Errorf("log errx formatted: %{reason}",
		errx.Errf("ooops with %{extra}", "value"))

	log.Errorf("log errx formatted with user field: %{reason}",
		errx.With("field", 1).Errf("ooops with %{extra}", "value"))

	log.Errorf("log errx formatted with ecs field: %{reason}",
		errx.With(ecs.Host.Hostname("localhost")).Errf("ooops with %{extra}", "value"))

	log.Errorf("log errx verbose formatted: %{+reason}",
		errx.Errf("ooops with %{extra}", "value"))

	log.Errorf("wrap EOF error with location: %v",
		errx.Wrap(io.EOF, "failed to read %{file}", "file.txt"))

	log.Errorf("wrap EOF twice: %v",
		errx.Wrap(
			errx.Wrap(io.EOF, "unepxected end of file in %{file}", "file.txt"),
			"error reading files in %{dir}", "path/to/files"))

	log.Errorf("wrap EOF with Errf: %v",
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
	).Errorf("wrap unexpected EOF with additional fields: %v",
		errx.With(
			ecs.File.Path("file.txt"),
			ecs.File.Extension("txt"),
			ecs.File.Owner("me"),
		).Wrap(io.EOF, "failed to read file"))

	log.With(
		"test", "field",
	).Errorf("Can not open keystore: %{error}",
		errx.With("op", "db/open").Wrap(
			errx.With("op", "db/init").Wrap(io.EOF,
				"failed to read db header in %{file}", "dbname/file.db"),
			"can not open database %{database}", "dbname"))

	log.Errorf("many errors: %v",
		errx.WrapAll([]error{
			io.EOF,
			io.ErrClosedPipe,
		}, "init operation failed"))

	log.Errorf("wrapped many errors tree: %v",
		errx.WrapAll([]error{
			errx.Wrap(io.EOF, "unexpected eof in %{file}", "tx.log"),
			errx.Wrap(io.ErrClosedPipe, "remote connection to %{server} closed", "localhost"),
		}, "init operation failed"))

	log.Errorf("multiple errors: %v || %v",
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
