// Copyright 2021 Upbound Inc
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package dep

import (
	"fmt"
	"strings"

	"github.com/crossplane/crossplane/apis/pkg/v1beta1"
)

const (
	packageTagFmt = "%s:%s"
)

// New returns a new v1beta1.Dependency based on the given package name
// and PackageType (reprented as a string).
// Expects names of the form source@version where @version can be
// left blank in order to indicate 'latest'.
func New(pkg, t string) v1beta1.Dependency {
	// if the passed in ver was blank use the default to pass
	// constraint checks and grab latest semver
	version := defaultVer

	ps := strings.Split(pkg, "@")

	source := ps[0]
	if len(ps) == 2 {
		version = ps[1]
	}

	d := v1beta1.Dependency{
		Package:     source,
		Type:        v1beta1.ProviderPackageType,
		Constraints: version,
	}

	if strings.Title(strings.ToLower(t)) == string(v1beta1.ConfigurationPackageType) {
		d.Type = v1beta1.ConfigurationPackageType
	}

	return d
}

// ImgTag returns the full image tag "source:version" of the given dependency
func ImgTag(d v1beta1.Dependency) string {
	// NOTE(@tnthornton) this should ONLY be used after the version constraint
	// has been resolved for the given dependency. Using a semver range is not
	// a valid tag format and will cause lookups to this string to fail.
	return fmt.Sprintf(packageTagFmt, d.Identifier(), d.Constraints)
}
