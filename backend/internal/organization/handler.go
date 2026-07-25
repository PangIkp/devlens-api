package organization

// Handler owns HTTP-facing organization behavior.
type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}
