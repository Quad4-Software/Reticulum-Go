{{ range .Pages -}}
`[{{ .Title | safeHTML }}`{{ .RelPermalink }}]{{ with .Description }} {{ . }}{{ end }}
{{ end -}}
