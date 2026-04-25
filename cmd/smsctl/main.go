package main

import (
	"fmt"
	"os"

	"github.com/S-martin-7/sms/internal/config"
	"github.com/S-martin-7/sms/internal/db"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd := os.Args[1]
	rest := os.Args[2:]
	var err error
	switch cmd {
	case "migrate":
		err = runMigrate(rest)
	case "admin":
		err = runAdmin(rest)
	case "tenant":
		err = runTenant(rest)
	case "key":
		err = runKey(rest)
	case "inbound":
		err = runInbound(rest)
	case "-h", "--help", "help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `smsctl — SMS gateway admin CLI

Commands:
  migrate up|down|version
  admin create --email X --password Y [--role superadmin|operator]
  admin list
  tenant create --name "Acme" [--daily-limit N]
  tenant list
  tenant suspend --id N
  tenant activate --id N
  key issue --tenant-id N [--name "label"]
  key list --tenant-id N
  key revoke --id N
  inbound assign --msisdn 569... --tenant-id N [--label "..."]
  inbound list
  inbound unassign --msisdn 569...`)
}

func runMigrate(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: smsctl migrate up|down|version")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	switch args[0] {
	case "up":
		if err := db.Migrate(cfg.DatabaseURL, "up"); err != nil {
			return fmt.Errorf("migrate up: %w", err)
		}
		fmt.Println("migrate up: ok")
	case "down":
		if err := db.Migrate(cfg.DatabaseURL, "down"); err != nil {
			return fmt.Errorf("migrate down: %w", err)
		}
		fmt.Println("migrate down: ok")
	case "version":
		v, dirty, err := db.Version(cfg.DatabaseURL)
		if err != nil {
			return err
		}
		fmt.Printf("version=%d dirty=%t\n", v, dirty)
	default:
		return fmt.Errorf("unknown migrate subcommand: %s", args[0])
	}
	return nil
}
