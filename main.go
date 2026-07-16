package main

import (
	"bufio"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Pipeline struct {
	ID   int
	Name string
}

type CacheItem struct {
	Data      []Pipeline
	StoredAt  time.Time
	ExpiresAt time.Time
}

type Metrics struct {
	TotalReadRequests   int
	DatabaseReadQueries int
	CacheHits           int
	CacheMisses         int
	CacheExpirations    int
	TotalResponseTime   time.Duration
}

var (
	db          *sql.DB
	cache       *CacheItem
	cacheTTL    = 30 * time.Second
	fakeDBDelay = 200 * time.Millisecond
	metrics     Metrics
	reader      = bufio.NewReader(os.Stdin)
)

func main() {
	var err error

	db, err = sql.Open("sqlite", "cache_lab.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatal(err)
	}

	setupDatabase()

	fmt.Println("Cache Lab started.")
	fmt.Println("This demo compares direct database fetching with cached fetching.")
	fmt.Println("The cache is a single in-process teaching example, not a production cache.")
	fmt.Println()

	for {
		printMenu()

		choice := readInput("Choose an option: ")

		switch choice {
		case "1":
			fetchDirectlyFromDB()
		case "2":
			fetchUsingCache()
		case "3":
			fetchUsingCacheThreeTimes()
		case "4":
			waitForTTL()
		case "5":
			updateDatabaseWithoutClearingCache()
		case "6":
			updateDatabaseAndClearCache()
		case "7":
			clearCache()
		case "8":
			showMetrics()
		case "9":
			fmt.Println("Exiting Cache Lab. Goodbye.")
			return
		default:
			fmt.Println("Invalid option. Try again.")
		}

		fmt.Println()
	}
}

func printMenu() {
	fmt.Println("====================================")
	fmt.Println("CACHE LAB")
	fmt.Println("====================================")
	fmt.Println("1. Fetch directly from database")
	fmt.Println("2. Fetch using cache")
	fmt.Println("3. Fetch using cache 3 times")
	fmt.Println("4. Wait for TTL to expire")
	fmt.Println("5. Update database WITHOUT clearing cache")
	fmt.Println("6. Update database AND clear cache")
	fmt.Println("7. Clear cache manually")
	fmt.Println("8. Show metrics summary")
	fmt.Println("9. Exit")
	fmt.Println("====================================")
}

func setupDatabase() {
	createTableQuery := `
	CREATE TABLE IF NOT EXISTS pipelines (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL
	);`

	if _, err := db.Exec(createTableQuery); err != nil {
		log.Fatal(err)
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM pipelines").Scan(&count); err != nil {
		log.Fatal(err)
	}

	if count == 0 {
		seedData := []string{
			"New Lead",
			"Contacted",
			"Demo Scheduled",
			"Proposal Sent",
			"Closed Won",
		}

		for _, name := range seedData {
			if _, err := db.Exec("INSERT INTO pipelines (name) VALUES (?)", name); err != nil {
				log.Fatal(err)
			}
		}
	}
}

func fetchDirectlyFromDB() {
	start := time.Now()

	data := fetchPipelinesFromDB()
	duration := time.Since(start)

	metrics.TotalReadRequests++
	metrics.DatabaseReadQueries++
	metrics.TotalResponseTime += duration

	printResult("DATABASE", "NONE", 1, duration, data)
}

func fetchUsingCache() {
	start := time.Now()
	wasExpired := false
	now := time.Now()

	if cache != nil {
		if now.Before(cache.ExpiresAt) {
			duration := time.Since(start)

			metrics.TotalReadRequests++
			metrics.CacheHits++
			metrics.TotalResponseTime += duration

			printResult("CACHE", "HIT", 0, duration, cache.Data)
			fmt.Printf("TTL remaining: %s\n", time.Until(cache.ExpiresAt).Round(time.Second))
			return
		}

		wasExpired = true

		metrics.CacheExpirations++
	}

	data := fetchPipelinesFromDB()

	storedAt := time.Now()
	cache = &CacheItem{
		Data:      data,
		StoredAt:  storedAt,
		ExpiresAt: storedAt.Add(cacheTTL),
	}

	duration := time.Since(start)

	metrics.TotalReadRequests++
	metrics.CacheMisses++
	metrics.DatabaseReadQueries++
	metrics.TotalResponseTime += duration

	cacheStatus := "MISS"
	if wasExpired {
		cacheStatus = "EXPIRED"
	}

	printResult("DATABASE", cacheStatus, 1, duration, data)
	fmt.Printf("Cache stored for: %s\n", cacheTTL)
}

func fetchUsingCacheThreeTimes() {
	fmt.Println("Running cached fetch 3 times...")
	fmt.Println()

	for i := 1; i <= 3; i++ {
		fmt.Printf("Request #%d\n", i)
		fetchUsingCache()
		fmt.Println()
	}
}

func fetchPipelinesFromDB() []Pipeline {
	// This delay is intentional and exists only to make the demo visible.
	time.Sleep(fakeDBDelay)

	rows, err := db.Query("SELECT id, name FROM pipelines ORDER BY id")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	var pipelines []Pipeline

	for rows.Next() {
		var pipeline Pipeline

		if err := rows.Scan(&pipeline.ID, &pipeline.Name); err != nil {
			log.Fatal(err)
		}

		pipelines = append(pipelines, pipeline)
	}

	if err := rows.Err(); err != nil {
		log.Fatal(err)
	}

	return pipelines
}

func waitForTTL() {
	if cache == nil {
		fmt.Println("Cache is empty. There is no TTL to wait for.")
		return
	}

	remaining := time.Until(cache.ExpiresAt)

	if remaining <= 0 {
		fmt.Println("Cache entry has already expired.")
		return
	}

	wait := remaining + time.Second
	fmt.Printf("Waiting %s for cache TTL to expire...\n", wait.Round(time.Second))
	time.Sleep(wait)
	fmt.Println("TTL expired. Try fetching with cache again.")
}

func updateDatabaseWithoutClearingCache() {
	newName := fmt.Sprintf("New Pipeline %d", time.Now().Unix())

	if _, err := db.Exec("INSERT INTO pipelines (name) VALUES (?)", newName); err != nil {
		log.Fatal(err)
	}

	fmt.Println("Database updated.")
	fmt.Println("New pipeline added:", newName)
	fmt.Println("Cache was NOT cleared.")
	fmt.Println("If cache still has valid data, the next cached fetch may return stale data.")
}

func updateDatabaseAndClearCache() {
	newName := fmt.Sprintf("Fresh Pipeline %d", time.Now().Unix())

	if _, err := db.Exec("INSERT INTO pipelines (name) VALUES (?)", newName); err != nil {
		log.Fatal(err)
	}

	cache = nil

	fmt.Println("Database updated.")
	fmt.Println("New pipeline added:", newName)
	fmt.Println("Cache was cleared.")
	fmt.Println("Next cached fetch will be a cache miss and will fetch fresh data.")
}

func clearCache() {
	cache = nil
	fmt.Println("Cache cleared manually.")
}

func showMetrics() {
	fmt.Println("METRICS SUMMARY")
	fmt.Println("------------------------------------")
	fmt.Printf("Total read requests:        %d\n", metrics.TotalReadRequests)
	fmt.Printf("Database read queries:      %d\n", metrics.DatabaseReadQueries)
	fmt.Printf("Cache hits:                 %d\n", metrics.CacheHits)
	fmt.Printf("Cache misses:               %d\n", metrics.CacheMisses)
	fmt.Printf("Expired entries encountered:%d\n", metrics.CacheExpirations)

	if metrics.TotalReadRequests > 0 {
		avg := metrics.TotalResponseTime / time.Duration(metrics.TotalReadRequests)
		fmt.Printf("Average read response:      %s\n", avg)
	}

	fmt.Println("------------------------------------")
}

func printResult(source string, cacheStatus string, dbQueries int, duration time.Duration, data []Pipeline) {
	fmt.Println("RESULT")
	fmt.Println("------------------------------------")
	fmt.Printf("Source used:       %s\n", source)
	fmt.Printf("Cache status:      %s\n", cacheStatus)
	fmt.Printf("Database queries:  %d\n", dbQueries)
	fmt.Printf("Response time:     %s\n", duration)
	fmt.Println("Data:")

	for _, item := range data {
		fmt.Printf("  - %d: %s\n", item.ID, item.Name)
	}

	fmt.Println("------------------------------------")
}

func readInput(label string) string {
	fmt.Print(label)

	input, err := reader.ReadString('\n')
	if err != nil {
		log.Fatal(err)
	}

	return strings.TrimSpace(input)
}
