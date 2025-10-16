package com.example.oomtest.config;

import io.swagger.v3.oas.models.OpenAPI;
import io.swagger.v3.oas.models.info.Contact;
import io.swagger.v3.oas.models.info.Info;
import io.swagger.v3.oas.models.info.License;
import io.swagger.v3.oas.models.servers.Server;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

import java.util.List;

@Configuration
public class OpenApiConfig {

    @Bean
    public OpenAPI customOpenAPI() {
        return new OpenAPI()
                .info(new Info()
                        .title("OOM 测试应用 API")
                        .description("用于测试 JVM OOM 的 Spring Boot 应用，提供内存管理、监控和测试功能")
                        .version("1.0.0")
                        .contact(new Contact()
                                .name("OOM Test Team")
                                .email("support@example.com")
                                .url("https://github.com/example/oom-test"))
                        .license(new License()
                                .name("MIT License")
                                .url("https://opensource.org/licenses/MIT")))
                .servers(List.of(
                        new Server()
                                .url("http://localhost:8080")
                                .description("本地开发环境"),
                        new Server()
                                .url("http://localhost:8085")
                                .description("Docker 容器环境")
                ));
    }
}
