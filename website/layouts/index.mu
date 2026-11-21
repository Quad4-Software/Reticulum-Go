{{ partial "reticulum/header" . }}
{{ partial "reticulum/nav" . }}
{{- if not (findRE `(?m)^#\s` .RawContent) }}
> {{ partial "doc-title" . }}
{{ end }}
{{ partial "reticulum/body" . }}
{{- if site.Data.docsnav }}
{{ partial "reticulum/doclist" . }}
{{- else }}
{{ partial "reticulum/pagelist" . }}
{{- end }}
{{ partial "reticulum/footer" . }}
