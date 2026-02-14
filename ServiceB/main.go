package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var validCepRegex = regexp.MustCompile(`^\d{8}$`)

type CepAwesomeapiResponse struct {
	Cep         string `json:"cep"`
	AddressType string `json:"address_type"`
	AddressName string `json:"address_name"`
	Address     string `json:"address"`
	State       string `json:"state"`
	District    string `json:"district"`
	Latitude    string `json:"lat"`
	Longitude   string `json:"lng"`
	City        string `json:"city"`
	Ibge        string `json:"city_ibge"`
	Ddd         string `json:"ddd"`
}

type CurrentUnits struct {
	Time          string `json:"time"`
	Interval      string `json:"interval"`
	Temperature2M string `json:"temperature_2m"`
}

type Current struct {
	Time          string  `json:"time"`
	Interval      int     `json:"interval"`
	Temperature2M float64 `json:"temperature_2m"`
}

type WeatherApiResponse struct {
	Latitude             float64      `json:"latitude"`
	Longitude            float64      `json:"longitude"`
	GenerationtimeMs     float64      `json:"generationtime_ms"`
	UtcOffsetSeconds     int          `json:"utc_offset_seconds"`
	Timezone             string       `json:"timezone"`
	TimezoneAbbreviation string       `json:"timezone_abbreviation"`
	Elevation            float64      `json:"elevation"`
	CurrentUnits         CurrentUnits `json:"current_units"`
	Current              Current      `json:"current"`
}

type Temperature struct {
	City  string  `json:"city"`
	TempC float64 `json:"temp_C"`
	TempF float64 `json:"temp_F"`
	TempK float64 `json:"temp_K"`
}

func initProvider() (func(context.Context) error, error) {
	ctx := context.Background()

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName("serviceb"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	conn, err := grpc.NewClient("otel-collector:4317", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC connection to collector: %w", err)
	}

	traceExporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithGRPCConn(conn))
	if err != nil {
		return nil, fmt.Errorf("failed to create trace exporter: %w", err)
	}

	bsp := sdktrace.NewBatchSpanProcessor(traceExporter)
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithResource(res),
		sdktrace.WithSpanProcessor(bsp),
	)
	otel.SetTracerProvider(tracerProvider)

	otel.SetTextMapPropagator(propagation.TraceContext{})

	return tracerProvider.Shutdown, nil
}

func main() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	shutdown, err := initProvider()
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := shutdown(ctx); err != nil {
			log.Printf("failed to shutdown TracerProvider: %s", err)
		}
	}()

	router := chi.NewRouter()

	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(middleware.Recoverer)
	router.Use(middleware.Logger)
	router.Use(middleware.Timeout(60 * time.Second))
	// promhttp
	router.Handle("/metrics", promhttp.Handler())
	router.Get("/{cep}", HandlerCep)

	go func() {
		log.Println("Starting server on port 8090")
		if err := http.ListenAndServe(":8090", router); err != nil {
			log.Fatal(err)
		}
	}()

	select {
	case <-sigCh:
		log.Println("Shutting down gracefully, CTRL+C pressed...")
	case <-ctx.Done():
		log.Println("Shutting down due to other reason...")
	}
}

func HandlerCep(w http.ResponseWriter, r *http.Request) {
	carrier := propagation.HeaderCarrier(r.Header)
	ctx := r.Context()
	ctx = otel.GetTextMapPropagator().Extract(ctx, carrier)

	tracer := otel.Tracer("microservice-tracer")
	ctx, span := tracer.Start(ctx, "HandlerCep")
	defer span.End()

	cep := chi.URLParam(r, "cep")
	if cep == "" || !validCepRegex.MatchString(cep) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid zipcode"})
		return
	}

	cepResponse, err := CepAwesomeapi(ctx, cep)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "can not find zipcode"})
		return
	}

	weatherResponse, err := WeatherApi(ctx, cepResponse.Latitude, cepResponse.Longitude)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "can not find zipcode"})
		return
	}

	tempC := weatherResponse.Current.Temperature2M
	result := Temperature{
		City:  cepResponse.City,
		TempC: tempC,
		TempF: tempC*1.8 + 32,
		TempK: tempC + 273.15,
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(result)
}

func CepAwesomeapi(ctx context.Context, cep string) (*CepAwesomeapiResponse, error) {
	tracer := otel.Tracer("microservice-tracer")
	ctx, span := tracer.Start(ctx, "CepAwesomeapi")
	defer span.End()

	url := "https://cep.awesomeapi.com.br/json/" + cep
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("cep not found")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cep api returned %d", resp.StatusCode)
	}

	var cepResponse CepAwesomeapiResponse
	if err := json.Unmarshal(body, &cepResponse); err != nil {
		return nil, err
	}
	// AwesomeAPI pode retornar 200 com mensagem de erro no body para CEP inválido
	if cepResponse.Cep == "" {
		return nil, fmt.Errorf("cep not found")
	}
	return &cepResponse, nil
}

func WeatherApi(ctx context.Context, latitude, longitude string) (*WeatherApiResponse, error) {
	tracer := otel.Tracer("microservice-tracer")
	ctx, span := tracer.Start(ctx, "WeatherApi")
	defer span.End()

	// Valida se latitude e longitude são numéricos válidos
	if _, err := strconv.ParseFloat(latitude, 64); err != nil {
		return nil, fmt.Errorf("invalid coordinates: %w", err)
	}
	if _, err := strconv.ParseFloat(longitude, 64); err != nil {
		return nil, fmt.Errorf("invalid coordinates: %w", err)
	}

	url := fmt.Sprintf("https://api.open-meteo.com/v1/forecast?latitude=%s&longitude=%s&current=temperature_2m", latitude, longitude)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("weather api returned %d", resp.StatusCode)
	}

	var weatherResponse WeatherApiResponse
	if err := json.Unmarshal(body, &weatherResponse); err != nil {
		return nil, err
	}
	return &weatherResponse, nil
}
