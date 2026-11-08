package dto

type CollectionCreateReq struct {
	RequestKey string `json:"requestKey" binding:"required"`
	Mode       string `json:"mode" binding:"required"`
}
type ChangeCreateReq struct {
	RequestKey      string `json:"requestKey" binding:"required"`
	SourceDeviceID  int    `json:"sourceDeviceId"`
	TargetDeviceIDs []int  `json:"targetDeviceIds"`
	Kind            string `json:"kind"`
	VNI             int    `json:"vni"`
	Direction       string `json:"direction"`
}
