package main

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"digitdns/internal/crypto"
	"digitdns/internal/database"
)

// runStorageCommand handles the two maintenance commands that need a bbolt
// handle. They run before the daemon opens the store, so both get the exclusive
// lock bbolt requires: db-stats only reads, but a live daemon still holds that
// lock, so a maintenance window is needed for either command.
func runStorageCommand(args []string, dbPath string, cipher *crypto.Cipher, out, errOut io.Writer) (bool, int) {
	if out == nil {
		out = io.Discard
	}
	if errOut == nil {
		errOut = io.Discard
	}
	if len(args) == 0 {
		return false, 0
	}

	switch strings.ToLower(args[0]) {
	case "db-stats":
		stats, err := database.InspectFile(dbPath)
		if err != nil {
			fmt.Fprintln(errOut, err)
			return true, 1
		}
		printDBStats(out, "Database", stats)
		return true, 0
	case "compact-db":
		before, after, err := database.CompactFile(dbPath, cipher)
		if err != nil {
			fmt.Fprintln(errOut, err)
			return true, 1
		}
		printDBStats(out, "Before", before)
		printDBStats(out, "After", after)
		if before.SizeBytes > 0 {
			reclaimed := 100 * float64(before.SizeBytes-after.SizeBytes) / float64(before.SizeBytes)
			fmt.Fprintf(out, "Reclaimed %.1f%% of the database file.\n", reclaimed)
		}
		return true, 0
	default:
		return false, 0
	}
}

func printDBStats(out io.Writer, label string, stats database.FileStats) {
	fmt.Fprintf(out, "%s: %s\n", label, stats.Path)
	fmt.Fprintf(out, "  size:       %d bytes\n", stats.SizeBytes)
	fmt.Fprintf(out, "  free:       %d bytes across %d pages\n", stats.FreeBytes, stats.FreePages)
	names := make([]string, 0, len(stats.Buckets))
	for name := range stats.Buckets {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Fprintf(out, "  bucket %s: %d keys\n", name, stats.Buckets[name])
	}
}
