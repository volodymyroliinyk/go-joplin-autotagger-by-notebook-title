package main

import (
    "bytes"
    "encoding/json"
    "fmt"
    "io"
    "log"
    "net/http"
    "net/url"
    "os"
    "strings"
    "time"
)

// TODO:[1] unit testing.
// === CONFIGURATION (MUST REPLACE) ===

// API URL of your local Joplin instance (default 41184)
const JOPLIN_API_BASE = "http://localhost:41184"

// Web Clipper API token found in Joplin settings
// Paste your token here or use an environment variable.

// PREFIX FOR ALL TAGS CREATED BASED ON NOTEBOOK NAMES
const TAG_PREFIX = "notebook."

// Maximum number of request attempts in case of error
const maxRetries = 3

// === DATA STRUCTURES FOR API ===

// Structure for the notation file (Folder in API)
type Folder struct {
    ID    string `json:"id"`
    Title string `json:"title"`
}

// Structure for the tag
type Tag struct {
    ID    string `json:"id"`
    Title string `json:"title"`
}

// Structure for the note
type Note struct {
    ID       string `json:"id"`
    Title    string `json:"title"`
    ParentID string `json:"parent_id"` // ID of parent notebook (folder)
}

// General structure for paginated result
type PaginatedResponse struct {
    Items      json.RawMessage `json:"items"`
    HasMore    bool            `json:"has_more"`
    TotalItems int             `json:"total_items"`
}

// === API UTILITIES ===

// bufferToReadCloser wraps bytes.Buffer to reuse the request body.
func bufferToReadCloser(buf *bytes.Buffer) io.ReadCloser {
    if buf == nil {
        return io.NopCloser(bytes.NewBuffer(nil))
    }
    // Create a copy of the buffer for a new request to avoid the "body already read" error
    return io.NopCloser(bytes.NewBuffer(buf.Bytes()))
}

// makeAPIRequest makes an HTTP request with authentication and retry logic
func makeAPIRequest(method, endpoint string, body *bytes.Buffer, token string) ([]byte, error) {
    u, err := url.Parse(JOPLIN_API_BASE + endpoint)
    if err != nil {
        return nil, fmt.Errorf("URL parsing error: %w", err)
    }

    q := u.Query()
    q.Set("token", token)
    u.RawQuery = q.Encode()
    fullURL := u.String()

    for i := 0; i < maxRetries; i++ {
        var requestBody io.Reader
        if body != nil {
            requestBody = bufferToReadCloser(body)
        }

        req, err := http.NewRequest(method, fullURL, requestBody)
        if err != nil {
            return nil, fmt.Errorf("request creation error: %w", err)
        }
        if body != nil {
            req.Header.Set("Content-Type", "application/json")
        }

        client := &http.Client{Timeout: 10 * time.Second}
        resp, err := client.Do(req)
        if err != nil {
            log.Printf("Error executing request to %s (trying %d): %v", fullURL, i+1, err)
            time.Sleep(time.Second * time.Duration(1<<i))
            continue
        }
        defer resp.Body.Close()

        respBody, _ := io.ReadAll(resp.Body)

        if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
            // Because Joplin returns 500 if the tag exists, but this is not a critical error for us.
            // We just don't update the map, and move on to the next tag.
            errorString := string(respBody)
            if strings.Contains(errorString, "already exists") {
                return nil, fmt.Errorf("tag already exists: %s", errorString)
            }

            return nil, fmt.Errorf("API error. Status: %s (%d). Respond: %s", resp.Status, resp.StatusCode, respBody)
        }

        return respBody, nil
    }

    return nil, fmt.Errorf("request failed after %d attempts", maxRetries)
}

// fetchAll fetches all elements from the paginated endpoint
func fetchAll[T any](endpoint string, token string) ([]T, error) {
    var allItems []T
    page := 1
    limit := 100

    baseEndpoint := endpoint
    if !strings.Contains(baseEndpoint, "?") {
        baseEndpoint += "?"
    } else {
        baseEndpoint += "&"
    }

    for {
        // Since `endpoint` already contains initial fields, we only add pagination
        pagedEndpoint := fmt.Sprintf("%s%d&page=%d", baseEndpoint, limit, page)

        respBody, err := makeAPIRequest("GET", pagedEndpoint, nil, token)
        if err != nil {
            return nil, err
        }

        var pagedResponse PaginatedResponse
        if err := json.Unmarshal(respBody, &pagedResponse); err != nil {
            return nil, fmt.Errorf("paginated response parsing error: %w", err)
        }

        var items []T
        if err := json.Unmarshal(pagedResponse.Items, &items); err != nil {
            return nil, fmt.Errorf("element parsing error: %w", err)
        }

        allItems = append(allItems, items...)

        if !pagedResponse.HasMore {
            break
        }
        page++
    }
    log.Printf("... Total %d items loaded.", len(allItems))
    return allItems, nil
}

// === BASIC SCRIPT LOGIC ===

func main() {
    log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
    fmt.Println("=== START: Automatically Tagging Joplin Notes ===")
    fmt.Printf("The tag prefix to use: %s\n", TAG_PREFIX)

    token := os.Getenv("JOPLIN_TOKEN")
    if token == "" {
        log.Fatal("ERROR: Environment variable JOPLIN_TOKEN is not set or empty.")
    }

    // 1. GETTING NOTEBOOKS
    fmt.Println("\n--- 1. Downloading all notebooks and collecting unique titles ---")
    folders, err := fetchAll[Folder]("/folders?fields=id,title", token)
    if err != nil {
        log.Fatalf("Critical error when loading notebooks: %v", err)
    }

    // Notepad ID Map -> TAG Name (normalized, for ID lookup)
    folderIDToNormalizedTagName := make(map[string]string)
    // Unique set of TAG NAMES we want to create (with original case)
    requiredTagNames := make(map[string]struct{})

    for _, f := range folders {
        prefixedTagName := TAG_PREFIX + f.Title

        normalizedTagName := strings.ToLower(prefixedTagName)

        folderIDToNormalizedTagName[f.ID] = normalizedTagName
        requiredTagNames[prefixedTagName] = struct{}{}
    }
    fmt.Printf("Found %d notebooks. You need to create %d unique tags (with a prefix).\n", len(folders), len(requiredTagNames))

    // 2. OBTAINING TAGS
    fmt.Println("\n--- 2. Loading existing tags ---")
    existingTags, err := fetchAll[Tag]("/tags?fields=id,title", token)
    if err != nil {
        log.Fatalf("Critical error while loading tags: %v", err)
    }

    // Map NORMALIZED Tag name -> ID (for quick existence check)
    normalizedTagNameToID := make(map[string]string)
    for _, t := range existingTags {
        normalizedTagNameToID[strings.ToLower(t.Title)] = t.ID
    }
    fmt.Printf("Found %d existing tags.\n", len(existingTags))

    // 3. CREATING MISSING TAGS
    fmt.Println("\n--- 3. Creating tags corresponding to notebook names (with a prefix) ---")
    tagsCreated := 0

    // We store the IDs of new tags in a new map, because we did not find them in point 2

    for originalName := range requiredTagNames {
        normalizedName := strings.ToLower(originalName)

        // Check if a normalized name exists among existing tags
        if _, exists := normalizedTagNameToID[normalizedName]; exists {
            // If the tag already exists, we just skip it because its ID is already in normalizedTagNameToID
            continue
        }

        fmt.Printf("... Create a tag: %s\n", originalName)

        newTagData := map[string]string{"title": originalName}
        body, _ := json.Marshal(newTagData)

        respBody, err := makeAPIRequest("POST", "/tags", bytes.NewBuffer(body), token)

        if err != nil {
            // The log here will show that the tag already exists (if an error from the API)
            // Now, if the tag exists, we know it, but we don't know its ID.
            // If it wasn't found in #2, but caused an "already exists" error here,
            // this means it was created by another process, or there was an earlier error in the logic.
            // In order not to fail, we skip this tag, but we do not add the ID to newlyCreatedTagsID.
            log.Printf("ERROR CREATING TAG '%s': %v. The tag will be skipped in the next step.", originalName, err)
            continue
        }

        var newTag Tag
        if err := json.Unmarshal(respBody, &newTag); err != nil {
            log.Printf("Error parsing new tag: %v. We continue.", err)
            continue
        }

        // Add the ID of the newly created tag using its normalized name
        normalizedTagNameToID[normalizedName] = newTag.ID
        tagsCreated++
    }

    fmt.Printf("Finished creating tags. Created by: %d.\n", tagsCreated)

    // 4. RECEIVING NOTES
    fmt.Println("\n--- 4. Download all notes ---")
    // The tags field is no longer requested to avoid SQLITE_ERROR
    notes, err := fetchAll[Note]("/notes?fields=id,title,parent_id", token)
    if err != nil {
        log.Fatalf("Critical error while loading notes: %v", err)
    }
    fmt.Printf("Loaded %d notes for processing.\n", len(notes))

    // 5. TAGGING NOTES
    fmt.Println("\n--- 5. Applying tags to notes ---")
    tagsApplied := 0

    for _, note := range notes {
        // 1. Find the required NORMALIZED tag name
        normalizedTagName, exists := folderIDToNormalizedTagName[note.ParentID]
        if !exists {
            // The notes are probably in the root directory or the ID didn't match.
            continue
        }

        // 2. Find the ID of the tag using the normalized name
        requiredTagID, exists := normalizedTagNameToID[normalizedTagName]
        if !exists {
            // !!! FIX: Tag should have been found or created in 2/3. If not, this is a critical error in logic.
            log.Printf("Error: Could not find tag id for normalized name: %s. We skip the note: %s.", normalizedTagName, note.Title)
            continue
        }

        // 3. Apply the tag.
        fmt.Printf("... Tagging note: '%s' with ID tag '%s'\n", note.ID, requiredTagID)

        // Endpoint for binding a tag to a note: POST /tags/:tagId/notes
        taggingEndpoint := fmt.Sprintf("/tags/%s/notes", requiredTagID)

        // The request body contains only the ID of the note
        tagNoteData := map[string]string{"id": note.ID}
        body, _ := json.Marshal(tagNoteData)

        _, err := makeAPIRequest("POST", taggingEndpoint, bytes.NewBuffer(body), token)
        if err != nil {
            // If tagging failed (for example, a network error), log in and continue.
            log.Printf("Error tagging note '%s': %v. We continue.", note.Title, err)
            continue
        }

        tagsApplied++
    }

    fmt.Printf("\n=== COMPLETED ===\n")
    fmt.Printf("New tags have been created (with a prefix): %d\n", tagsCreated)
    fmt.Printf("Tags successfully applied:%d\n", tagsApplied)
    fmt.Println("The script completed successfully. Check out Joplin.")
}
