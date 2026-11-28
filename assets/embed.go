package assets

import _ "embed"

//go:embed ozevpn.ico
var IconApp []byte

//go:embed ozevpn-idle.ico
var IconIdle []byte

//go:embed ozevpn-connecting.ico
var IconConnecting []byte

//go:embed ozevpn-connected.ico
var IconConnected []byte

//go:embed mascot-ui.png
var BrandPNG []byte
