{{- $p := .Site.Params.reticulum -}}
{{- if $p.show_nav }}
{{- $menu := index .Site.Menus $p.nav_menu }}
{{- if $menu }}
{{ range $menu }}`[{{ .Name | safeHTML }}`{{ .URL }}]
{{ end }}
{{- else }}
`[{{ .Site.Home.Title | safeHTML }}`{{ .Site.Home.RelPermalink }}]
{{ range .Site.Sections }}
`[{{ .Title | safeHTML }}`{{ .RelPermalink }}]
{{ end }}
{{- end }}
-
{{ end }}
