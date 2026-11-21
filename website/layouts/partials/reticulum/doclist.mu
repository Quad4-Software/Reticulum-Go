{{- range hugo.Data.docsnav.groups -}}
{{- range .pages -}}
{{- $p := site.GetPage (printf "/docs/%s" .slug) -}}
{{- if $p }}
`[{{ or .label (partial "doc-title" $p) | safeHTML }}`{{ printf "/docs/%s/" .slug }}]
{{- end -}}
{{- end -}}
{{- end }}
