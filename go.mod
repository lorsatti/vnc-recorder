module github.com/lorsatti/vnc-recorder

go 1.14

replace github.com/amitbet/vnc2video v0.0.0-20190616012314-9d50b9dab1d9 => github.com/gyuchang/vnc2video v0.0.0-20200605054616-a77a2a01a317

require (
	github.com/amitbet/vnc2video v0.0.0-20190616012314-9d50b9dab1d9
	github.com/icza/mjpeg v0.0.0-20170217094447-85dfbe473743 // indirect
	github.com/sirupsen/logrus v1.6.0
	github.com/urfave/cli v1.22.4
	golang.org/x/sys v0.0.0-20200728102440-3e129f6d46b1 // indirect
)
