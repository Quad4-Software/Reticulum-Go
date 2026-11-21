{{- $p := .Site.Params.reticulum -}}
-
{{ with $p.footer }}{{ . }}
{{ end }}
