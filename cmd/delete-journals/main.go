package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"gocrud/internal/models"
	"gocrud/internal/store"
)

func main() {
	// Parse command-line flags
	dryRun := flag.Bool("dry-run", false, "Show what would be deleted without actually deleting")
	collection := flag.String("collection", "", "Firestore collection name (defaults to FIRESTORE_COLLECTION env var or 'healthstuff')")
	database := flag.String("database", "healthbase", "Firestore database name (defaults to 'healthbase')")
	verbose := flag.Bool("verbose", false, "Show verbose information about items in the collection")
	flag.Parse()

	logger := log.New(os.Stdout, "delete-journals ", log.LstdFlags|log.Lmicroseconds)
	ctx := context.Background()

	logger.Printf("Starting journal deletion utility")
	if *dryRun {
		logger.Printf("DRY RUN MODE: No items will be deleted")
	}

	// Determine database name
	databaseName := *database
	if databaseName == "" {
		databaseName = os.Getenv("FIRESTORE_DATABASE")
		if databaseName == "" {
			databaseName = "healthbase"
		}
	}
	// Set database name in environment for the store to pick up
	os.Setenv("FIRESTORE_DATABASE", databaseName)

	// Determine collection name
	collectionName := *collection
	if collectionName == "" {
		collectionName = os.Getenv("FIRESTORE_COLLECTION")
		if collectionName == "" {
			collectionName = "healthstuff"
		}
	}
	logger.Printf("Using Firestore database: %s", databaseName)
	logger.Printf("Using Firestore collection: %s", collectionName)

	// Check authentication setup
	projectID := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if projectID == "" {
		projectID = os.Getenv("GCLOUD_PROJECT")
	}
	if projectID == "" {
		projectID = os.Getenv("GCP_PROJECT")
	}
	if projectID == "" {
		projectID = os.Getenv("PROJECT_ID")
	}
	if projectID != "" {
		logger.Printf("Using project ID: %s", projectID)
	} else {
		logger.Printf("WARN: No project ID found in environment variables (GOOGLE_CLOUD_PROJECT, GCLOUD_PROJECT, GCP_PROJECT, PROJECT_ID)")
		logger.Printf("      Project ID will be resolved from default credentials or metadata server")
	}

	credsPath := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	if credsPath != "" {
		logger.Printf("Using credentials from: %s", credsPath)
	} else {
		logger.Printf("INFO: GOOGLE_APPLICATION_CREDENTIALS not set, using default credentials")
		logger.Printf("      (from gcloud auth application-default login or service account)")
	}

	// Create Firestore store
	fsStore, err := store.NewFirestoreStoreWithCollection(ctx, logger, collectionName)
	if err != nil {
		logger.Fatalf("FATAL: failed to initialize Firestore store: %v", err)
	}
	defer func() {
		if err := fsStore.Close(); err != nil {
			logger.Printf("ERROR: failed to close Firestore client: %v", err)
		}
	}()

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Query all journals
	logger.Printf("Querying all journals (Type == %q)...", models.ItemTypeJournal)
	items, err := fsStore.ListItems(ctx, models.ItemTypeJournal, nil, nil, nil)
	if err != nil {
		logger.Printf("ERROR: Failed to list journals: %v", err)
		logger.Printf("\n=== Authentication Troubleshooting ===")
		logger.Printf("Permission denied errors usually indicate:")
		logger.Printf("1. Missing or invalid credentials")
		logger.Printf("2. Insufficient IAM permissions for Firestore")
		logger.Printf("\nTo fix:")
		logger.Printf("- Authenticate with: gcloud auth application-default login")
		logger.Printf("- Or set GOOGLE_APPLICATION_CREDENTIALS to a service account key file")
		logger.Printf("- Ensure your account/service account has 'Cloud Datastore User' role")
		logger.Printf("  (roles/datastore.user) or 'Firestore User' role")
		logger.Printf("- Verify the project ID is correct")
		logger.Printf("- Verify the database name '%s' exists in the project", databaseName)
		logger.Fatalf("\nFATAL: failed to list journals: %v", err)
	}

	totalCount := len(items)
	logger.Printf("Found %d journal(s) to delete", totalCount)

	if totalCount == 0 {
		logger.Printf("No journals found with Type == %q", models.ItemTypeJournal)

		// If verbose, list all items in the collection to help diagnose
		if *verbose {
			logger.Printf("\n=== Verbose Mode: Listing all items in collection ===")
			allItems, err := fsStore.ListItems(ctx, "", nil, nil, nil)
			if err != nil {
				logger.Printf("ERROR: Failed to list all items: %v", err)
			} else {
				logger.Printf("Total items in collection '%s': %d", collectionName, len(allItems))
				if len(allItems) > 0 {
					logger.Printf("\nItem types found:")
					typeCounts := make(map[string]int)
					for _, item := range allItems {
						typeCounts[item.Type]++
					}
					for itemType, count := range typeCounts {
						logger.Printf("  - %s: %d item(s)", itemType, count)
					}
					logger.Printf("\nFirst 10 items (ID, Type, Name):")
					maxShow := 10
					if len(allItems) < maxShow {
						maxShow = len(allItems)
					}
					for i := 0; i < maxShow; i++ {
						logger.Printf("  [%d] ID=%q, Type=%q, Name=%q", i+1, allItems[i].ID, allItems[i].Type, allItems[i].Name)
					}
				}
			}
		} else {
			logger.Printf("Tip: Use -verbose flag to see all items in the collection")
		}
		logger.Printf("Exiting.")
		return
	}

	// Confirm deletion unless in dry-run mode
	if !*dryRun {
		fmt.Printf("\nWARNING: This will delete %d journal(s) from collection '%s'.\n", totalCount, collectionName)
		fmt.Print("Type 'yes' to confirm: ")
		var confirmation string
		fmt.Scanln(&confirmation)
		if confirmation != "yes" {
			logger.Printf("Deletion cancelled by user")
			return
		}
	}

	// Delete each journal
	deletedCount := 0
	failedCount := 0

	for i, item := range items {
		// Check for interrupt signal
		select {
		case <-sigChan:
			logger.Printf("\nReceived interrupt signal. Stopping deletion.")
			logger.Printf("Deleted %d/%d journals before interruption", deletedCount, totalCount)
			os.Exit(1)
		default:
		}

		logger.Printf("[%d/%d] Deleting journal: ID=%q, Name=%q", i+1, totalCount, item.ID, item.Name)

		if *dryRun {
			logger.Printf("  [DRY RUN] Would delete: %s", item.ID)
			deletedCount++
		} else {
			if err := fsStore.DeleteItem(ctx, item.ID); err != nil {
				logger.Printf("  ERROR: Failed to delete journal %s: %v", item.ID, err)
				failedCount++
			} else {
				logger.Printf("  Successfully deleted: %s", item.ID)
				deletedCount++
			}
		}
	}

	// Summary
	logger.Printf("\n=== Deletion Summary ===")
	logger.Printf("Total journals found: %d", totalCount)
	if *dryRun {
		logger.Printf("Would delete: %d", deletedCount)
	} else {
		logger.Printf("Successfully deleted: %d", deletedCount)
		if failedCount > 0 {
			logger.Printf("Failed to delete: %d", failedCount)
		}
	}
	logger.Printf("Deletion utility completed")
}
