package main

import (
	"context"
	"log"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/tapiaw38/globalstay-service-be/internal/adapters/datasources"
	"github.com/tapiaw38/globalstay-service-be/internal/adapters/web"
	"github.com/tapiaw38/globalstay-service-be/internal/adapters/web/integrations"
	"github.com/tapiaw38/globalstay-service-be/internal/platform/appcontext"
	"github.com/tapiaw38/globalstay-service-be/internal/platform/config"
	"github.com/tapiaw38/globalstay-service-be/internal/platform/migrations"
	"github.com/tapiaw38/globalstay-service-be/internal/platform/nosql"
	"github.com/tapiaw38/globalstay-service-be/internal/usecases"
)

func main() {
	scope := config.GetScope()

	log.Printf("scope identifier: %s", scope)

	if err := initConfig(); err != nil {
		panic(err)
	}

	if err := run(); err != nil {
		panic(err)
	}
}

func run() error {
	configService := config.GetConfigService()
	ctx := context.TODO()

	var clients []nosql.Client
	defer func() {
		for _, client := range clients {
			_ = client.Disconnect(ctx)
		}
	}()

	migrationsCLient, err := createAndDeferClient(configService.NoSQLConfig.Migrations, &clients)
	if err != nil {
		return err
	}

	hotelClient, err := createAndDeferClient(configService.NoSQLConfig.Hotels, &clients)
	if err != nil {
		return err
	}

	locationClient, err := createAndDeferClient(configService.NoSQLConfig.Locations, &clients)
	if err != nil {
		return err
	}

	if err := hotelClient.RunMigrations(
		migrationsCLient.GetCollection(),
		migrations.ExecuteHotelMigrations(hotelClient.GetCollection().Name()),
	); err != nil {
		log.Printf("Error running migrations for hotel: %v", err)
		return err
	}

	servicesClient, err := createAndDeferClient(configService.NoSQLConfig.Services, &clients)
	if err != nil {
		return err
	}

	reservationsClient, err := createAndDeferClient(configService.NoSQLConfig.Reservations, &clients)
	if err != nil {
		return err
	}

	if configService.ServerConfig.GinMode == config.DebugMode {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	app := gin.Default()
	ginConfig := cors.DefaultConfig()
	ginConfig.AllowOrigins = []string{"*"}
	ginConfig.AllowCredentials = true
	ginConfig.AllowMethods = []string{"*"}
	ginConfig.AllowHeaders = []string{"*"}
	ginConfig.ExposeHeaders = []string{"*"}
	app.Use(cors.New(ginConfig))

	bootstrap(app, hotelClient, locationClient, servicesClient, reservationsClient, &configService)

	return app.Run(":" + configService.ServerConfig.Port)
}

func bootstrap(
	app *gin.Engine,
	hotelClient nosql.Client,
	locationClient nosql.Client,
	servicesClient nosql.Client,
	reservationsClient nosql.Client,
	configService *config.ConfigurationService,
) {
	datasources := datasources.CreateDatasources(hotelClient, locationClient, servicesClient, reservationsClient)
	integrations := integrations.CreateIntegrations(configService)
	contextFactory := appcontext.NewFactory(datasources, integrations, configService)
	useCases := usecases.CreateUsecases(contextFactory)
	web.RegisterApplicationRoutes(app, useCases)
	web.RegisterJobRoutes(app, useCases)
}

func createAndDeferClient(config *config.NoSQLCollectionConfig, clients *[]nosql.Client) (nosql.Client, error) {
	client, err := nosql.NewClient(config)
	if err != nil {
		return nil, err
	}

	*clients = append(*clients, client)

	return client, nil
}
