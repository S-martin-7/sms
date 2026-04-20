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

func runTenant(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: smsctl tenant create|list|suspend|activate")
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
	case "create":
		fs := flag.NewFlagSet("tenant create", flag.ExitOnError)
		name := fs.String("name", "", "tenant name")
		daily := fs.Int("daily-limit", 0, "daily SMS limit (0=unlimited)")
		_ = fs.Parse(rest)
		if *name == "" {
			return fmt.Errorf("--name required")
		}
		in := tenancy.CreateTenantInput{Name: *name}
		if *daily > 0 {
			v := int32(*daily)
			in.DailySMSLimit = &v
		}
		t, err := svc.CreateTenant(ctx, in)
		if err != nil {
			return err
		}
		fmt.Printf("tenant created: id=%d name=%s status=%s\n", t.ID, t.Name, t.Status)
		return nil
	case "list":
		ts, err := svc.ListTenants(ctx)
		if err != nil {
			return err
		}
		for _, t := range ts {
			fmt.Printf("%d\t%s\t%s\t%s\n", t.ID, t.Name, t.Status, t.CreatedAt.Format(time.RFC3339))
		}
		return nil
	case "suspend", "activate":
		fs := flag.NewFlagSet("tenant "+sub, flag.ExitOnError)
		id := fs.Int64("id", 0, "tenant id")
		_ = fs.Parse(rest)
		if *id == 0 {
			return fmt.Errorf("--id required")
		}
		target := "suspended"
		if sub == "activate" {
			target = "active"
		}
		if err := svc.SetStatus(ctx, *id, target); err != nil {
			return err
		}
		fmt.Printf("tenant %d -> %s\n", *id, target)
		return nil
	default:
		return fmt.Errorf("unknown tenant subcommand: %s", sub)
	}
}
