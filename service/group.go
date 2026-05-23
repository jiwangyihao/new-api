package service

func GetUserUsableGroups(userGroup string) map[string]string {
	return nil
}

func GroupInUserUsableGroups(userGroup, groupName string) bool {
	return false
}

func GetUserAutoGroup(userGroup string) []string {
	return nil
}

func GetUserGroupRatio(userGroup, group string) float64 {
	return 1
}
