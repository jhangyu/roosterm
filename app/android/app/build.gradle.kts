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

    // Release signing comes from the environment (CI secrets or a local export). ci.py enforces
    // that CI always sets it; without it a local build stays debug-signed.
    val keystoreFile = providers.environmentVariable("ANDROID_KEYSTORE_FILE").orNull
    if (!keystoreFile.isNullOrEmpty()) {
        val secrets = listOf("ANDROID_KEYSTORE_PASSWORD", "ANDROID_KEY_ALIAS", "ANDROID_KEY_PASSWORD")
            .associateWith { providers.environmentVariable(it).orNull.orEmpty() }
        val missing = secrets.filterValues { it.isEmpty() }.keys
        if (missing.isNotEmpty()) {
            throw GradleException(
                "expected ANDROID_KEYSTORE_PASSWORD, ANDROID_KEY_ALIAS, ANDROID_KEY_PASSWORD " +
                    "with ANDROID_KEYSTORE_FILE=$keystoreFile, missing $missing"
            )
        }
        signingConfigs.create("release") {
            storeFile = file(keystoreFile)
            storePassword = secrets.getValue("ANDROID_KEYSTORE_PASSWORD")
            keyAlias = secrets.getValue("ANDROID_KEY_ALIAS")
            keyPassword = secrets.getValue("ANDROID_KEY_PASSWORD")
        }
    } else {
        logger.warn("ANDROID_KEYSTORE_FILE not set: release is DEBUG-signed and cannot update a release install")
    }

    buildTypes {
        release {
            signingConfig = signingConfigs.getByName(if (keystoreFile.isNullOrEmpty()) "debug" else "release")
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
