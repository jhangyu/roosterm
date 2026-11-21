plugins {
    id("com.android.application")
    // The Flutter Gradle Plugin must be applied after the Android and Kotlin Gradle plugins.
    id("dev.flutter.flutter-gradle-plugin")
}

android {
    namespace = "us.jhangy.roosterm"
    compileSdk = flutter.compileSdkVersion
    ndkVersion = flutter.ndkVersion

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    defaultConfig {
        applicationId = "us.jhangy.roosterm"
        minSdk = flutter.minSdkVersion
        targetSdk = flutter.targetSdkVersion
        versionCode = flutter.versionCode
        versionName = flutter.versionName
        // arm64 only, except the per-ABI split build (see ../build.gradle.kts); Flutter rejects
        // abiFilters together with ABI splits.
        if (project.findProperty("split-per-abi") != "true") {
            ndk { abiFilters += "arm64-v8a" }
        }
    }

    buildTypes {
        release {
            // Release is signed with the debug key on purpose: the app is sideloaded, not published.
            signingConfig = signingConfigs.getByName("debug")
        }
    }
}

kotlin {
    compilerOptions {
        jvmTarget = org.jetbrains.kotlin.gradle.dsl.JvmTarget.JVM_17
    }
}

flutter {
    source = "../.."
}
