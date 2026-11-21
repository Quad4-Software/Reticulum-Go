{{- $p := .Site.Params.reticulum -}}
{{ with $p.fg }}#!fg={{ . }}
{{ end }}{{ with $p.bg }}#!bg={{ . }}
{{ end }}{{ with $p.cache_ttl }}#!c={{ . }}
{{ end }}
