{{ partial "reticulum/header" . }}
{{ partial "reticulum/nav" . }}
{{- if not (findRE `(?m)^#\s` .RawContent) }}
> {{ partial "doc-title" . }}
{{ end }}
{{ with .Date }}
{{ .Format "2006-01-02" }}
{{ end }}
{{ partial "reticulum/body" . }}
{{ partial "reticulum/footer" . }}
