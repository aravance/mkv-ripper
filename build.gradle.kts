import com.fussionlabs.gradle.tasks.GoTask

plugins {
    alias(libs.plugins.go)
}

go {
    os = listOf("linux")
    arch = listOf("amd64")
    extraBuildArgs = listOf("-o", "mkv-ripper", "github.com/aravance/mkv-ripper/cmd/server")
    goVersion = "1.24.5"
}

val gobin = "${project.projectDir}/bin"
val templ = "$gobin/templ"

tasks.getByName("assemble").dependsOn("mkv-ripper")
tasks.register("mkv-ripper", GoTask::class) {
    group = "build"
    description = "Compile the mkv-ripper binary"
    dependsOn("templGenerate")
    outputs.file("build/mkv-ripper")
    inputs.files(fileTree(project.projectDir) { include("**/*.go") })
    goTaskArgs = mutableListOf("build", "-o", "build/mkv-ripper", "github.com/aravance/mkv-ripper/cmd/server")
}

tasks.getByName("clean").doLast {
    fileTree("view") { include("**/*_templ.go") }
        .forEach { it -> it.delete() }
}

tasks.register("installTempl", GoTask::class) {
    group = "build setup"
    description = "Install the templ generator binary"
    outputs.file(templ)
    goTaskEnv = mutableMapOf("GOBIN" to gobin)
    goTaskArgs = mutableListOf("install", "github.com/a-h/templ/cmd/templ@latest")
}

tasks.register("templGenerate", Exec::class) {
    group = "build"
    description = "Generate go code from .templ files"
    dependsOn("installTempl")
    executable(templ)
    args("generate")
    inputs.files(fileTree("view") { include("**/*.templ") })
    outputs.files(fileTree("view") { include("**/*.templ") }.map { it -> File(it.parent, it.name.replace(".templ", "_templ.go")) })
}
