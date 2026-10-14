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
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
	Rider       Rider  `json:"rider"`
}

type Message struct {
	Message string `json:"message"`
}
