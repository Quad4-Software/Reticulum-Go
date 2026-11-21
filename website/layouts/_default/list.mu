{{ partial "reticulum/header" . }}
{{ partial "reticulum/nav" . }}
> {{ .Title }}
{{ with .Description }}
{{ . }}
{{ end }}
{{ partial "reticulum/pagelist" . }}
{{ partial "reticulum/body" . }}
{{ partial "reticulum/footer" . }}
