{{ partial "reticulum/header" . }}
{{ partial "reticulum/nav" . }}
{{- if not (findRE `(?m)^#\s` .RawContent) }}
> {{ partial "doc-title" . }}
{{ end }}
{{ with .Description }}
{{ . }}
{{ end }}
{{ if eq .Section "docs" }}
{{ partial "reticulum/doclist" . }}
{{ else }}
{{ partial "reticulum/pagelist" . }}
{{ end }}
{{ partial "reticulum/body" . }}
{{ partial "reticulum/footer" . }}
