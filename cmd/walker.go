package main

import (
	_ "embed"
	"fmt"
	"log"
	"os"
	"os/signal"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/abenz1267/walker/internal/config"
	"github.com/abenz1267/walker/internal/modules"
	"github.com/abenz1267/walker/internal/state"
	"github.com/abenz1267/walker/internal/ui"
	"github.com/abenz1267/walker/internal/util"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

//go:embed version.txt
var version string

var now = time.Now().UnixMilli()

func main() {
	state := state.Get()

	defer func() {
		if state.IsService {
			os.Remove(modules.DmenuSocketAddrGet)
		}

		os.Remove(modules.DmenuSocketAddrReply)
	}()

	appName := "dev.benz.walker"

	var wg sync.WaitGroup

	if len(os.Args) > 1 {
		args := os.Args[1:]

		isNew := false

		if len(os.Args) > 0 {
			if slices.Contains(args, "-n") || slices.Contains(args, "--new") {
				isNew = true
			}

			if slices.Contains(args, "-y") || slices.Contains(args, "--password") {
				isNew = true
			}

			if slices.Contains(args, "-v") || slices.Contains(args, "--version") {
				fmt.Println(version)
				return
			}

			if slices.Contains(args, "-b") || slices.Contains(args, "--benchmark") {
				fmt.Println("Startup: ", now)
				state.Benchmark = true
			}

			state.IsService = slices.Contains(args, "--gapplication-service")

			if state.IsService {
				cfg := config.Get(state.ExplicitConfig)
				state.StartServiceableModules(cfg)
			}

			if slices.Contains(args, "-d") || slices.Contains(args, "--dmenu") {
				if !isNew && !state.IsService && util.FileExists(modules.DmenuSocketAddrGet) {
					wg.Add(1)

					dmenu := modules.Dmenu{}
					dmenu.Send()

					go func(wg *sync.WaitGroup) {
						dmenu.ListenForReply()
						wg.Done()
					}(&wg)
				}
			}

			if isNew {
				appName = fmt.Sprintf("%s-%d", appName, time.Now().Unix())
			}
		}
	}

	app := gtk.NewApplication(appName, gio.ApplicationHandlesCommandLine)

	app.AddMainOption("modules", 'm', glib.OptionFlagNone, glib.OptionArgString, "modules to be loaded", "the modules")
	app.AddMainOption("new", 'n', glib.OptionFlagNone, glib.OptionArgNone, "start new instance ignoring service", "")
	app.AddMainOption("keepsort", 'k', glib.OptionFlagNone, glib.OptionArgNone, "don't sort alphabetically", "")
	app.AddMainOption("password", 'y', glib.OptionFlagNone, glib.OptionArgNone, "launch in password mode", "")
	app.AddMainOption("dmenu", 'd', glib.OptionFlagNone, glib.OptionArgNone, "run in dmenu mode", "")
	app.AddMainOption("config", 'c', glib.OptionFlagNone, glib.OptionArgString, "config file to use", "")
	app.AddMainOption("theme", 's', glib.OptionFlagNone, glib.OptionArgString, "theme to use", "")
	app.AddMainOption("placeholder", 'p', glib.OptionFlagNone, glib.OptionArgString, "placeholder text", "")
	app.AddMainOption("query", 'q', glib.OptionFlagNone, glib.OptionArgString, "initial query", "")
	app.AddMainOption("labelcolumn", 'l', glib.OptionFlagNone, glib.OptionArgString, "column to use for the label", "")
	app.AddMainOption("separator", 't', glib.OptionFlagNone, glib.OptionArgString, "column separator", "")
	app.AddMainOption("version", 'v', glib.OptionFlagNone, glib.OptionArgNone, "print version", "")
	app.AddMainOption("forceprint", 'f', glib.OptionFlagNone, glib.OptionArgNone, "forces printing input if no item is selected", "")
	app.AddMainOption("bench", 'b', glib.OptionFlagNone, glib.OptionArgNone, "prints nanoseconds for start and displaying in both service and client", "")
	app.AddMainOption("active", 'a', glib.OptionFlagNone, glib.OptionArgString, "active item", "")

	app.Connect("activate", ui.Activate(state))

	app.ConnectCommandLine(func(cmd *gio.ApplicationCommandLine) int {
		if state.Benchmark {
			fmt.Println("start handle cmd: ", time.Now().UnixMilli())
		}

		options := cmd.OptionsDict()

		if options.Contains("bench") {
			state.Benchmark = true
		}

		modulesString := options.LookupValue("modules", glib.NewVariantString("").Type())
		configString := options.LookupValue("config", glib.NewVariantString("").Type())
		themeString := options.LookupValue("theme", glib.NewVariantString("").Type())
		placeholderString := options.LookupValue("placeholder", glib.NewVariantString("").Type())
		initialQueryString := options.LookupValue("query", glib.NewVariantString("").Type())

		if options.Contains("dmenu") {
			labelColumnString := options.LookupValue("labelcolumn", glib.NewVariantString("").Type())
			separatorString := options.LookupValue("separator", glib.NewVariantString("").Type())
			activeItemString := options.LookupValue("active", glib.NewVariantString("").Type())

			if separatorString != nil && separatorString.String() != "" {
				if state.Dmenu != nil {
					state.Dmenu.Separator = separatorString.String()
				} else {
					state.DmenuSeparator = separatorString.String()
				}
			}

			if labelColumnString != nil && labelColumnString.String() != "" {
				col, err := strconv.Atoi(labelColumnString.String())
				if err != nil {
					log.Panicln(err)
				}

				if state.Dmenu != nil {
					state.Dmenu.LabelColumn = col
				} else {
					state.DmenuLabelColumn = col
				}
			}

			if activeItemString != nil && activeItemString.String() != "" {
				n := activeItemString.String()

				a, err := strconv.Atoi(n)
				if err != nil {
					log.Println(err)
				}

				val := a - 1

				state.ActiveItem = &val
			}

			state.ExplicitModules = append(state.ExplicitModules, "dmenu")
			state.IsDmenu = true

		} else {
			if modulesString != nil && modulesString.String() != "" {
				m := strings.Split(modulesString.String(), ",")
				state.ExplicitModules = m
			}
		}

		state.ForcePrint = options.Contains("forceprint")
		state.Password = options.Contains("password")
		state.KeepSort = options.Contains("keepsort")

		if placeholderString != nil && placeholderString.String() != "" {
			state.ExplicitPlaceholder = placeholderString.String()
		}

		if initialQueryString != nil && initialQueryString.String() != "" {
			state.InitialQuery = initialQueryString.String()
		}

		if configString != nil && configString.String() != "" {
			state.ExplicitConfig = configString.String()
		}

		if themeString != nil && themeString.String() != "" {
			state.ExplicitTheme = themeString.String()
		}

		if state.Benchmark {
			fmt.Println("run activate: ", time.Now().UnixMilli())
		}

		app.Activate()
		cmd.Done()

		return 0
	})

	app.Flags()

	if state.IsService {
		app.Hold()

		signal_chan := make(chan os.Signal, 1)
		signal.Notify(signal_chan,
			syscall.SIGHUP,
			syscall.SIGINT,
			syscall.SIGTERM,
			syscall.SIGQUIT)

		go func() {
			for {
				<-signal_chan

				os.Remove(modules.DmenuSocketAddrGet)
				os.Remove(modules.DmenuSocketAddrReply)

				os.Exit(0)
			}
		}()
	}

	if state.Benchmark {
		fmt.Println("start run: ", time.Now().UnixMilli())
	}

	code := app.Run(os.Args)

	wg.Wait()

	if code > 0 {
		os.Exit(code)
	}
}
