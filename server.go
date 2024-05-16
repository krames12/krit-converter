package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
)

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse the multipart form
	err := r.ParseMultipartForm(10 << 20)
	if err != nil {
		http.Error(w, "Unable to parse form", http.StatusBadRequest)
		return
	}

	// Retrieve the form data
	text := r.FormValue("text")
	fontFile, header, err := r.FormFile("font")
	if err != nil {
		http.Error(w, "Error retrieving the font file", http.StatusInternalServerError)
		return
	}
	defer fontFile.Close()

	// Save the font file to a temporary file
	tempFontFile, err := os.Create(filepath.Join("uploads", header.Filename))
	if err != nil {
		http.Error(w, "Error creating the font file", http.StatusInternalServerError)
		return
	}
	defer tempFontFile.Close()

	_, err = io.Copy(tempFontFile, fontFile)
	if err != nil {
		http.Error(w, "Error saving the font file", http.StatusInternalServerError)
		return
	}

	// Create the output BMP file path
	bitmapPath := filepath.Join("uploads", text+".bmp")

	// Construct the ImageMagick command to create a bitmap
	convertCmd := exec.Command("convert",
		"-size", "100x100", "xc:white",
		"-font", tempFontFile.Name(),
		"-pointsize", "72",
		"-fill", "black",
		"-draw", fmt.Sprintf("text 10,70 '%s'", text),
		bitmapPath,
	)

	// Run the ImageMagick command
	err = convertCmd.Run()
	if err != nil {
		http.Error(w, "Error creating the bitmap image", http.StatusInternalServerError)
		return
	}

	// Create the output SVG file path
	svgPath := filepath.Join("uploads", text+".svg")

	// Construct the Potrace command to convert BMP to SVG
	potraceCmd := exec.Command("potrace", bitmapPath, "-s", "-o", svgPath)

	// Run the Potrace command
	err = potraceCmd.Run()
	if err != nil {
		http.Error(w, "Error converting bitmap to SVG", http.StatusInternalServerError)
		return
	}

	// Serve the SVG file directly to the client
	w.Header().Set("Content-Type", "image/svg+xml")
	http.ServeFile(w, r, svgPath)
}

func main() {
	http.HandleFunc("/upload", uploadHandler)

	// Serve static files from the current directory
	http.Handle("/", http.FileServer(http.Dir(".")))

	fmt.Println("Server started at :8080")
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		panic(err)
	}
}
