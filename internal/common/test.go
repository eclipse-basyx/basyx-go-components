// You can edit this code!
// Click here and start typing.
package common

import "strings"

func main() {
	str := "$aasdesc#specificAssetIds[].externalSubjectId.keys[].value"
	// 1. cut away everything before and including #
	str = str[strings.Index(str, "#")+1:]
	// 2. Split into array of strings at .
	parts := strings.Split(str, ".")
	// 3. print each part
	for _, part := range parts {
		println(part)
	}

}

type ArrayPart struct {
	Name  string
	Index int
}

type SimplePart struct {
	Name string
}

/*
specificAssetIds[1]
externalSubjectId
keys[3]
value
*/
