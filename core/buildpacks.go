package core

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/opentable/sous/tools/dir"
	"github.com/opentable/sous/tools/file"
	"github.com/opentable/sous/tools/version"
)

type Buildpacks []Buildpack

type Buildpack struct {
	Name, Desc          string
	StackVersions       *StackVersions
	DefaultStackVersion string
	Scripts             struct {
		Common, Base, Command, Compile, Detect, Test, ListBaseimage string
	}
}

type RunnableBuildpack struct {
	Buildpack
	DetectedStackVersionRange string
	ResolvedStackVersionRange *version.R
	StackVersion              *StackVersion
}

type RunnableBuildpacks []RunnableBuildpack

func (bps Buildpacks) Detect(dirPath string) RunnableBuildpacks {
	packs := RunnableBuildpacks{}
	for _, p := range bps {
		if rbp, err := p.Detect(dirPath); err == nil {
			packs = append(packs, *rbp)
		}
	}
	return packs
}

func (bps Buildpacks) Get(name string) (*Buildpack, bool) {
	for _, bp := range bps {
		if bp.Name == name {
			return &bp, true
		}
	}
	return nil, false
}

// BuildpackError represents errors in the configuration of the buildpack
// itself. E.g scripts that don't output expected error codes or the correct
// stdout data, or scripts whose stack version configuration doesn't make sense.
type BuildpackError struct {
	Buildpack       Buildpack
	Script, Message string
}

func (bpe BuildpackError) Error() string {
	m := bpe.Message
	if bpe.Script != "" {
		m = fmt.Sprintf("%s; %s", bpe.Script, m)
	}
	return fmt.Sprintf("buildpack %s: %s", bpe.Buildpack.Name, m)
}

func (bp Buildpack) ConfigErr(f string, a ...interface{}) BuildpackError {
	return bp.ScriptErr("", "misconfigured; "+f, a...)
}

func (bp Buildpack) ScriptErr(scriptName, f string, a ...interface{}) BuildpackError {
	message := fmt.Sprintf(f, a...)
	return BuildpackError{bp, scriptName, message}
}

func (bp Buildpack) Detect(dirPath string) (*RunnableBuildpack, error) {
	detected, err := bp.RunScript("detect.sh", bp.Scripts.Detect, dirPath)
	if err != nil {
		return nil, err
	}
	parts := strings.Split(detected, " ")
	if len(parts) != 2 || parts[0] != bp.Name {
		return nil, bp.ScriptErr("returned %q; want '%s <stackversion>' where <stackversion> is either 'default' or semver range",
			detected, bp.Name)
	}
	detectedVersionRange := parts[1]
	var stackVersionRange *version.R
	if detectedVersionRange == "default" {
		stackVersionRange, err = version.NewRange(bp.DefaultStackVersion)
		if err != nil {
			return nil, bp.ConfigErr("unable to parse default stack version %q as semver range: %s", bp.DefaultStackVersion, err)
		}
	} else {
		stackVersionRange, err = version.NewRange(detectedVersionRange)
		if err != nil {
			return nil, bp.ScriptErr("detect.sh", "unable to parse %q as semver range: %s", detectedVersionRange, err)
		}
	}

	stackVersion, err := bp.StackVersions.GetBestStackVersion(stackVersionRange)
	if err != nil {
		return nil, bp.ConfigErr("unable to determine stack version: %s", err)
	}

	runnable := &RunnableBuildpack{
		Buildpack:                 bp,
		DetectedStackVersionRange: detectedVersionRange,
		ResolvedStackVersionRange: stackVersionRange,
		StackVersion:              stackVersion,
	}

	return runnable, nil
}

func (bp Buildpack) RunScript(name, contents, inDir string) (string, error) {
	// Add common.sh and base.sh
	contents = fmt.Sprintf("%s\n\n# base.sh\n%s\n\n# %s\n%s\n",
		bp.Scripts.Common, bp.Scripts.Base, name, contents)

	path := filepath.Join(inDir, name)

	data := []byte(contents)
	file.Write(data, path)
	file.RemoveOnExit(path)

	stderr := &bytes.Buffer{}
	stdout := &bytes.Buffer{}
	combined := &bytes.Buffer{}

	teeout := io.MultiWriter(stdout, combined)
	teeerr := io.MultiWriter(stderr, combined)

	c := exec.Command(path)
	c.Dir = inDir
	c.Stdout = teeout
	c.Stderr = teeerr

	context := GetContext()
	c.Env = append(c.Env, context.BuildpackEnv().Flatten()...)

	if err := c.Start(); err != nil {
		return "", err
	}

	if err := c.Wait(); err != nil {
		return "", fmt.Errorf("Error: %s; output from %s:\n%s", err, name, combined.String())
	}

	return strings.Trim(stdout.String(), "\n\r\t "), nil
}

func (tc *TargetContext) BaseImage(dirPath, targetName string) (string, error) {
	bp := tc.Buildpack
	detected, err := bp.RunScript("detect.sh", bp.Scripts.Detect, dirPath)
	if err != nil {
		return "", err
	}
	parts := strings.Split(detected, " ")
	if len(parts) != 2 || parts[0] != bp.Name {
		return "", fmt.Errorf("detect.sh returned %q; want '%s <stackversion>' where <stackversion> is either 'default' or semver range",
			detected, bp.Name)
	}
	stackVersion := parts[1]
	if stackVersion == "default" {
		stackVersion = bp.DefaultStackVersion
	}
	image, ok := bp.StackVersions.GetBaseImageTag(stackVersion, targetName)
	if !ok {
		return "", fmt.Errorf("buildpack %s does not have a base image for version %s", bp.Name, stackVersion)
	}
	return image, nil
}

func ParseBuildpacks(baseDir string) (Buildpacks, error) {
	if !dir.Exists(baseDir) {
		return nil, fmt.Errorf("buildpack dir not found: %s", baseDir)
	}

	common, _ := file.ReadString(filepath.Join(baseDir, "common.sh"))

	packs := Buildpacks{}
	err := filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() || path == baseDir {
			return nil
		}
		pack, err := ParseBuildpack(path)
		if err != nil {
			return fmt.Errorf("error parsing buildpack at %s: %s", path, err)
		}
		pack.Name = info.Name()
		pack.Scripts.Common = common
		packs = append(packs, pack)
		return filepath.SkipDir
	})
	if err != nil {
		return nil, err
	}
	return packs, nil
}

func ParseBuildpack(baseDir string) (Buildpack, error) {
	p := Buildpack{}
	var err error
	read := func(filename string) string {
		path := filepath.Join(baseDir, filename)
		s, ok := file.ReadString(path)
		if !ok {
			err = fmt.Errorf("unable to read file %s", path)
		}
		return s
	}
	p.Scripts.Base = read("base.sh")
	p.Scripts.Command = read("command.sh")
	p.Scripts.Compile = read("compile.sh")
	p.Scripts.Detect = read("detect.sh")
	p.Scripts.Test = read("test.sh")
	return p, err
}
