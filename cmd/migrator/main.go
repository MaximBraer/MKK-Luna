package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/mysql"
	_ "github.com/golang-migrate/migrate/v4/source/file"

	"MKK-Luna/internal/config"
)

func main() {
	var (
		cmd  string
		step int
		ver  int
	)

	flag.StringVar(&cmd, "cmd", "up", "migrate command: up|down|steps|force|version")
	flag.IntVar(&step, "step", 0, "steps for cmd=steps (positive or negative)")
	flag.IntVar(&ver, "ver", 0, "version for cmd=force")
	flag.Parse()

	cfg, err := config.New()
	if err != nil {
		fatalf("load config: %v", err)
	}

	dsn := fmt.Sprintf(
		"%s:%s@tcp(%s:%d)/%s?parseTime=true&multiStatements=true",
		cfg.MySQL.User,
		cfg.MySQL.Password,
		cfg.MySQL.Host,
		cfg.MySQL.Port,
		cfg.MySQL.DBName,
	)

	m, err := migrate.New("file://migrations", "mysql://"+dsn)
	if err != nil {
		fatalf("migrate.New: %v", err)
	}

	switch cmd {
	case "up":
		err = m.Up()
	case "down":
		err = m.Down()
	case "steps":
		if step == 0 {
			fatalf("-step required for cmd=steps")
		}
		err = m.Steps(step)
	case "force":
		if ver == 0 {
			fatalf("-ver required for cmd=force")
		}
		err = m.Force(ver)
	case "version":
		v, dirty, e := m.Version()
		if errors.Is(e, migrate.ErrNilVersion) {
			fmt.Println("version: none")
			return
		}
		if e != nil {
			fatalf("version: %v", e)
		}
		fmt.Printf("version: %d dirty=%v\n", v, dirty)
		return
	default:
		fatalf("unknown cmd: %s", cmd)
	}

	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		fatalf("migrate %s: %v", cmd, err)
	}

	fmt.Printf("migrate %s: ok\n", cmd)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
