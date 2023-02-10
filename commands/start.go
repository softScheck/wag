package commands

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/NHAS/wag/internal/config"
	"github.com/NHAS/wag/internal/data"
	"github.com/NHAS/wag/internal/router"
	"github.com/NHAS/wag/internal/webserver"
	"github.com/NHAS/wag/pkg/control/server"
	"github.com/NHAS/wag/ui"
)

type start struct {
	fs         *flag.FlagSet
	config     string
	noIptables bool
}

func Start() *start {
	gc := &start{
		fs: flag.NewFlagSet("start", flag.ContinueOnError),
	}

	gc.fs.StringVar(&gc.config, "config", "./config.json", "Configuration file location")

	gc.fs.Bool("noiptables", false, "Do not add iptables rules")

	return gc
}

func (g *start) FlagSet() *flag.FlagSet {
	return g.fs
}

func (g *start) Name() string {

	return g.fs.Name()
}

func (g *start) PrintUsage() {
	fmt.Println("Usage of start:")
	fmt.Println("  Start wag server (does not daemonise)")
	g.fs.PrintDefaults()
}

func (g *start) Check() error {
	g.fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "noiptables":
			g.noIptables = true
		}
	})

	err := config.Load(g.config)
	if err != nil {
		return err
	}

	err = data.Load(config.Values().DatabaseLocation)
	if err != nil {
		return fmt.Errorf("cannot load database: %v", err)
	}

	return nil

}

func (g *start) Run() error {

	error := make(chan error)

	if _, err := os.Stat(wag_was_upgraded); err == nil {
		os.Remove(wag_was_upgraded)
		g.noIptables = true
		log.Println("Wag was upgraded to", config.Version, " iptables will not be configured. (Due to presence of", wag_was_upgraded, ")")
	}

	err := router.Setup(error, !g.noIptables)
	if err != nil {
		return fmt.Errorf("unable to start router: %v", err)
	}
	defer func() {
		if !(strings.Contains(err.Error(), "listen unix") && strings.Contains(err.Error(), "address already in use")) {
			router.TearDown()
		}
	}()

	err = server.StartControlSocket()
	if err != nil {
		return fmt.Errorf("unable to create control socket: %v", err)
	}
	defer func() {

		if !(strings.Contains(err.Error(), "listen unix") && strings.Contains(err.Error(), "address already in use")) {
			server.TearDown()
		}
	}()

	err = webserver.Start(error)
	if err != nil {
		return fmt.Errorf("unable to start webserver: %v", err)
	}

	ui.StartWebServer(error)

	go func() {
		cancel := make(chan os.Signal, 1)
		signal.Notify(cancel, syscall.SIGTERM, syscall.SIGINT, os.Interrupt, syscall.SIGQUIT)

		s := <-cancel

		log.Printf("Got signal %s gracefully exiting\n", s)

		error <- errors.New("ignore me I am signal")
	}()

	log.Println("Wag started successfully, Ctrl + C to stop")
	err = <-error
	if err != nil && !strings.Contains(err.Error(), "ignore me I am signal") {
		return err
	}

	return nil
}
