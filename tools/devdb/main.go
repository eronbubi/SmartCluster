// Command devdb runs a local Postgres 18 for development without installing
// anything: binaries come from the embedded-postgres cache, data lives in
// ./.data/devdb. Development only, never shipped.
//
//	go run ./tools/devdb
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
)

func main() {
	port := flag.Uint("port", 54329, "port on 127.0.0.1")
	dir := flag.String("dir", filepath.Join(".data", "devdb"), "data directory")
	flag.Parse()

	if *port == 0 || *port > 65535 {
		log.Fatal("-port must be between 1 and 65535")
	}
	abs, err := filepath.Abs(*dir)
	if err != nil {
		log.Fatal(err)
	}
	pg := embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().
		Version(embeddedpostgres.V18).
		Port(uint32(*port)). //nolint:gosec // G115: checked to be at most 65535 above
		RuntimePath(filepath.Join(abs, "runtime")).
		DataPath(filepath.Join(abs, "data")).
		Username("smartcluster").Password("dev").Database("smartcluster").
		StartTimeout(2 * time.Minute))
	if err := pg.Start(); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("\nPostgres is running. Use:\n  SC_DATABASE_URL=postgres://smartcluster:dev@127.0.0.1:%d/smartcluster?sslmode=disable\nPress Ctrl+C to stop.\n", *port)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	if err := pg.Stop(); err != nil {
		log.Fatal(err)
	}
}
