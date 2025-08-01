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

tasks.getByName("assemble").dependsOn("mkv-ripper")
tasks.register("mkv-ripper", GoTask::class) {
    group = "build"
    description = "Compile the mkv-ripper binary"
    dependsOn("templGenerate")
    outputs.file("build/mkv-ripper")
    inputs.files(fileTree(project.projectDir) { include("**/*.go") })
    goTaskArgs = mutableListOf("build", "-o", "build/mkv-ripper", "github.com/aravance/mkv-ripper/cmd/server")
}

tasks.register("templGenerate", GoTask::class) {
    group = "build"
    description = "Generate go code from .templ files"
    inputs.files(fileTree("view") { include("**/*.templ") })
    outputs.files(fileTree("view") { include("**/*.templ") }.map { it -> File(it.parent, it.name.replace(".templ", "_templ.go")) })
    goTaskArgs = mutableListOf("tool", "github.com/a-h/templ/cmd/templ", "generate")
}

tasks.getByName("clean").doLast {
    fileTree("view") { include("**/*_templ.go") }
    .forEach { it -> it.delete() }
}
