package main

import (
	"os"
	"path"
	"strings"
	"time"

	"github.com/rjeczalik/notify"
)

// Reloader triggers a reload
type Reloader interface {
	Reload(paths []string)
}

const batchTime = time.Millisecond * 200

// This function batches events up, and emits just a list of paths for files
// considered changed. It applies some heuristics to deal with short-lived
// temporary files.
func batch(ch chan notify.EventInfo) chan []string {
	ret := make(chan []string)
	go func() {
		emap := make(map[string]bool)
		for {
			select {
			case evt := <-ch:
				emap[evt.Path()] = true
			case <-time.After(batchTime):
				if len(emap) > 0 {
					keys := make([]string, 0, len(emap))
					for k := range emap {
						_, err := os.Stat(k)
						if err == nil {
							keys = append(keys, k)
						}
					}
					ret <- keys
					emap = make(map[string]bool)
				}
			}
		}
	}()
	return ret
}

func watch(p string, ch chan notify.EventInfo) error {
	stat, err := os.Stat(p)
	if err != nil {
		return err
	}
	if stat.IsDir() {
		p = path.Join(p, "...")
	}
	return notify.Watch(p, ch, notify.All)
}

// Watch watches an endpoint for changes, if it supports them.
func (r Route) Watch(ch chan []string) error {
	switch r.Endpoint.(type) {
	case *filesystemEndpoint:
		ep := *r.Endpoint.(*filesystemEndpoint)
		evtchan := make(chan notify.EventInfo, 1)
		err := watch(string(ep), evtchan)
		if err != nil {
			return err
		}
		go func() {
			for files := range batch(evtchan) {
				for i, fpath := range files {
					files[i] = path.Join(
						r.Path,
						strings.TrimPrefix(
							fpath,
							string(ep),
						),
					)
				}
				ch <- files
			}
		}()
	}
	return nil
}

func liveEvents(lr Reloader, ch chan []string) {
	for ei := range ch {
		lr.Reload(ei)
	}
}

// WatchPaths watches a set of paths, and broadcasts changes through reloader.
func WatchPaths(paths []string, reloader Reloader) error {
	ch := make(chan []string, 1)
	for _, path := range paths {
		evtchan := make(chan notify.EventInfo, 1)
		err := watch(path, evtchan)
		if err != nil {
			return err
		}
		go func() {
			for files := range batch(evtchan) {
				ch <- files
			}
		}()
	}
	go liveEvents(reloader, ch)
	return nil
}

// WatchRoutes watches the route collection, and broadcasts changes through reloader.
func WatchRoutes(routes routeCollection, reloader Reloader) error {
	c := make(chan []string, 1)
	for i := range routes {
		err := routes[i].Watch(c)
		if err != nil {
			return err
		}
	}
	go liveEvents(reloader, c)
	return nil
}
