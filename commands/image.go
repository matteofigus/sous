package commands

import (
	"github.com/opentable/sous/build"
	"github.com/opentable/sous/tools/cli"
)

func ImageHelp() string {
	return `sous image prints the last built image tag for this project`
}

func Image(packs []*build.Pack, args []string) {
	target := "build"
	if len(args) != 0 {
		target = args[0]
	}
	_, context, _ := AssembleFeatureContext(target, packs)
	if context.BuildNumber() == 0 {
		cli.Fatalf("no builds yet")
	}
	cli.Outf(context.DockerTag())
	cli.Success()
}