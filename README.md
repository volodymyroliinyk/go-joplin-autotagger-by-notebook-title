# go-joplin-autotagger-by-notebook-title

---

# Joplin Auto Tagger By Notebook Title (Golang)

- This is a simple [Go](https://go.dev/) script that automatically tags your notes in [Joplin](https://joplinapp.org/)
  by notebook titles.
- The script uses the https://joplinapp.org/help/apps/clipper/ API and is designed to provide consistent tagging of
  large collections of notes.
-

## Basic capabilities

- Complete handling of collections: Uses pagination to correctly load all notes, tags and notebooks, regardless of their
  number (more than 100).
- Smart Search: Performs a case-insensitive search.
- Parses all notebook names.
- Creates tags based on notebook name with a special prefix Notebook `"Some notebook title"` to Tag
  `notebook.some notebook title`.
- Tags notes in notebook `"Some notebook title"` with corresponding tag `notebook.some notebook title`.

## Installation and configuration

1. Setting up the Joplin API
    - Before running the script, you must activate the [Joplin Web Clipper](https://joplinapp.org/help/apps/clipper/)
      API and obtain your token:
        - Open the [Joplin Desktop](https://joplinapp.org/help/install/#desktop-applications) application.
        - Go to Tools -> Options -> Web Clipper.
        - Enable the "Enable Web Clipper Service" option.
        - Copy the Authorization token from there.
        - Make sure Joplin Desktop is running when you run the script.
2. Go script configuration
    - Open the main.go file and replace the stub with your real token:
    ```
    const JOPLIN_API_BASE = "http://localhost:41184"
    const JOPLIN_TOKEN = "YOUR_COPIED_API_TOKEN"
    ```
3. Launch
    - Make sure you have Go (Golang) installed.
    - Save the file as main.go.
    - Open a terminal in the directory with the file.
    - Run the script: \
      ```go run main.go```

TODO:

- unit testing