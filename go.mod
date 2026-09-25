module github.com/TomoBossi/sensordemo

go 1.27.1

require github.com/TomoBossi/sensord v0.0.0

require (
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/term v0.46.0 // indirect
)

replace github.com/TomoBossi/sensord => ../sensord
