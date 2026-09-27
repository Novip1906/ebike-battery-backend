package serializer

type RiderCredentialsRequest struct {
	Login    string `json:"login" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type Rider struct {
	ID                    uint   `json:"id"`
	Login                 string `json:"login"`
	IsBikeServiceEngineer bool   `json:"is_bike_service_engineer"`
}

type RiderLoginResponse struct {
	Rider   Rider  `json:"rider"`
	Message string `json:"message"`
}

type Message struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}
