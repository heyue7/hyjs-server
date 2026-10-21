package constUtil

// AdminPlayerEditError 向后台明确返回可由运营人员处理的角色编辑业务约束。
type AdminPlayerEditError struct {
	Message string
}

func (e *AdminPlayerEditError) Error() string { return e.Message }
