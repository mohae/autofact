// Code generated by "stringer -type=Bucket"; DO NOT EDIT

package db

import "fmt"

const _Bucket_name = "InvalidClientRoleGroupClusterDatacenter"

var _Bucket_index = [...]uint8{0, 7, 13, 17, 22, 29, 39}

func (i Bucket) String() string {
	if i < 0 || i >= Bucket(len(_Bucket_index)-1) {
		return fmt.Sprintf("Bucket(%d)", i)
	}
	return _Bucket_name[_Bucket_index[i]:_Bucket_index[i+1]]
}
