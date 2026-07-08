package openapi

import "fmt"

// DocsHTML returns a self-contained documentation page rendering the spec at
// specURL with Scalar (https://github.com/scalar/scalar).
func DocsHTML(title, specURL string) []byte {
	return fmt.Appendf(nil, `<!doctype html>
<html>
<head>
  <title>%s</title>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
</head>
<body>
  <script id="api-reference" data-url="%s"></script>
  <script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"></script>
</body>
</html>`, title, specURL)
}
