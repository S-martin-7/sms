package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/S-martin-7/sms/internal/admin"
	"github.com/S-martin-7/sms/internal/config"
	"github.com/S-martin-7/sms/internal/db"
)

func runAdmin(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: smsctl admin create|list")
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
	svc := admin.NewService(pool, cfg.BcryptCost)

	switch sub {
	case "create":
		fs := flag.NewFlagSet("admin create", flag.ExitOnError)
		email := fs.String("email", "", "email")
		password := fs.String("password", "", "password")
		role := fs.String("role", "", "superadmin|operator (default auto)")
		_ = fs.Parse(rest)
		if *email == "" || *password == "" {
			return fmt.Errorf("--email and --password are required")
		}
		chosen := *role
		if chosen == "" {
			n, _ := svc.CountAdmins(ctx)
			if n == 0 {
				chosen = "superadmin"
			} else {
				chosen = "operator"
			}
		}
		u, err := svc.CreateAdmin(ctx, *email, *password, chosen)
		if err != nil {
			return err
		}
		fmt.Printf("admin created: id=%d email=%s role=%s\n", u.ID, u.Email, u.Role)
		return nil
	case "list":
		rows, err := pool.Query(ctx, `SELECT id, email, role, created_at FROM admin_users ORDER BY id`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			var email, role string
			var created time.Time
			if err := rows.Scan(&id, &email, &role, &created); err != nil {
				return err
			}
			fmt.Printf("%d\t%s\t%s\t%s\n", id, email, role, created.Format(time.RFC3339))
		}
		return rows.Err()
	default:
		return fmt.Errorf("unknown admin subcommand: %s", sub)
	}
}
