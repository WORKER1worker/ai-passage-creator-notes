# Spring Boot 3.5 基础

Spring Boot 是一个用于简化 Spring 应用程序开发的框架。它使得创建独立的、生产级的 Spring 应用程序变得非常快速和简单。下面是一些 Spring Boot 3.5 的基本概念，以及在本项目中使用的依赖注入、注解和项目结构的简单介绍。

## 依赖注入（Dependency Injection）

依赖注入是一个核心的设计模式，它允许你将对象的依赖关系从代码中移除，改为通过外部的方式提供给对象。Spring Boot 提供了一种非常强大的方式来管理这些依赖，通过 `@Autowired` 注解，可以轻松地将需要的依赖注入到类中。例如：

```java
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.stereotype.Service;

@Service
public class MyService {
    private final MyRepository myRepository;

    @Autowired
    public MyService(MyRepository myRepository) {
        this.myRepository = myRepository;
    }
}
```

在这个例子中，`MyService` 依赖于 `MyRepository`，Spring Boot 通过构造器注入的方式将 `MyRepository` 实例传递给 `MyService`。

## 注解（Annotations）

Spring Boot 使用注解来提供配置和设置信息。常见的注解包括：

- `@SpringBootApplication`：标记一个类为 Spring Boot 应用程序的主入口。
- `@RestController`：用于定义一个控制器，以处理 HTTP 请求。
- `@Service` 和 `@Repository`：分别用于定义服务层类和数据访问层类。

例如：

```java
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RestController;

@RestController
public class MyController {
    @GetMapping("/hello")
    public String sayHello() {
        return "Hello, World!";
    }
}
```

## 项目结构（Project Structure）

在 Spring Boot 应用程序中，项目的基本结构往往是：

```
/my-spring-boot-app
    ├── src
    │   ├── main
    │   │   ├── java
    │   │   │   └── com
    │   │   │       └── example
    │   │   │           └── demo
    │   │   │               ├── MyApplication.java
    │   │   │               ├── controller
    │   │   │               │   └── MyController.java
    │   │   │               └── service
    │   │   │                   └── MyService.java
    │   │   └── resources
    │   │       ├── application.properties
    │   │       └── static
    │   └── test
    └── pom.xml
```

- `MyApplication.java`：应用程序的主入口。
- `controller` 目录：包含处理请求的控制器。
- `service` 目录：包含业务逻辑的服务类。
- `resources` 目录：包含配置文件以及其他资源。

通过这样的项目结构，可以清晰地组织代码，提高代码的可读性和可维护性。