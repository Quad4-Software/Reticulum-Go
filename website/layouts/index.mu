{{ partial "reticulum/header" . }}
{{ partial "reticulum/nav" . }}
> {{ .Title }}
{{ partial "reticulum/body" . }}
{{ partial "reticulum/pagelist" .Site.Home }}
{{ partial "reticulum/footer" . }}
