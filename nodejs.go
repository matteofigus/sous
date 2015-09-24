package main

type NodePackage struct {
	Name    string
	Version string
	Engines NodePackageEngines
	Scripts NodePackageScripts
}
type NodePackageEngines struct {
	Node, NPM string
}
type NodePackageScripts struct {
	Start, Test string
}

func tryBuildNodeJS(bc *BuildContext) {
	var np *NodePackage
	if !readFileJSON(&np, "package.json") {
		return
	}
	buildInfo := buildNodeJS(bc, np)
	dief("Successfully built %s v%s as %s",
		bc.CanonicalPackageName(),
		buildInfo.Version,
		buildInfo.DockerImage())

}

func buildNodeJS(bc *BuildContext, np *NodePackage) *BuildInfo {
	return &BuildInfo{
		Context: bc,
		Version: np.Version,
	}
}
