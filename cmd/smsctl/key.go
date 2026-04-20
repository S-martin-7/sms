package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/S-martin-7/sms/internal/config"
	"github.com/S-martin-7/sms/internal/db"
	"github.com/S-martin-7/sms/internal/tenancy"
)

func runKey(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: smsctl key issue|list|revoke")
	}
	sub, rest := args[0], args[1:]

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("db: %w", err)
	}
	defer pool.Close()
	svc := tenancy.NewService(pool)

	switch sub {
	case "issue":
		fs := flag.NewFlagSet("key issue", flag.ExitOnError)
		tid := fs.Int64("tenant-id", 0, "tenant id")
		name := fs.String("name", "", "label")
		_ = fs.Parse(rest)
		if *tid == 0 {
			return fmt.Errorf("--tenant-id required")
		}
		issued, err := svc.IssueAPIKey(ctx, *tid, *name, cfg.APIKeyPepper)
		if err != nil {
			return err
		}
		fmt.Println("API KEY (shown ONCE - copy now):")
		fmt.Println(issued.Token)
		fmt.Printf("\nid=%d prefix=%s tenant_id=%d\n", issued.Record.ID, issued.Record.Prefix, issued.Record.TenantID)
		return nil
	case "list":
		fs := flag.NewFlagSet("key list", flag.ExitOnError)
		tid := fs.Int64("tenant-id", 0, "tenant id")
		_ = fs.Parse(rest)
		if *tid == 0 {
			return fmt.Errorf("--tenant-id required")
		}
		keys, err := svc.ListAPIKeys(ctx, *tid)
		if err != nil {
			return err
		}
		for _, k := range keys {
			state := "active"
			if k.RevokedAt != nil {
				state = "revoked@" + k.RevokedAt.Format(time.RFC3339)
			}
			name := ""
			if k.Name != nil {
				name = *k.Name
			}
			fmt.Printf("%d\t%s\t%s\t%s\n", k.ID, k.Prefix, name, state)
		}
		return nil
	case "revoke":
		fs := flag.NewFlagSet("key revoke", flag.ExitOnError)
		id := fs.Int64("id", 0, "key id")
		_ = fs.Parse(rest)
		if *id == 0 {
			return fmt.Errorf("--id required")
		}
		if err := svc.RevokeAPIKey(ctx, *id); err != nil {
			return err
		}
		fmt.Printf("key %d revoked\n", *id)
		return nil
	default:
		return fmt.Errorf("unknown key subcommand: %s", sub)
	}
}
