# Agostle
Agostle is a kind of _apostle_ - converts everything to PDF.
Everything:

  * Text, Spreadsheet, HTML and other office-like documents with the help of LibreOffice,
  * Images with GraphicsMagick,
  * Email with agostle (by traversing the tree and applying the transformations as needed).

# Install

    go get github.com/tgulacsi/agostle


# Usage
Agostle can be used for converting files, or start a HTTP server on port 8500, and respond
to requests like `/email/convert`.

# Build
The `requirements.txt` contains the needed programs, and a Dockerfile is present for Docker users, to be able to have a converter with every needed program installed, without polluting your environment.

Build agostle and the tgulacsi/agostle container:

	  go run make.go
