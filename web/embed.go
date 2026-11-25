// Package web embarque le front compilé (web/dist, produit par `npm run build`)
// dans le binaire.
package web

import "embed"

//go:embed all:dist
var Dist embed.FS
