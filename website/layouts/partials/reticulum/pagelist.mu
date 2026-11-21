{{ range .Pages -}}
`[{{ partial "doc-title" . | safeHTML }}`{{ .RelPermalink }}]{{ with .Description }} {{ . }}{{ end }}
{{ end -}}
