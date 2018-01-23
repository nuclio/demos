// Databricks notebook source
import org.apache.spark.sql.functions._
import org.apache.spark.sql.streaming.Trigger
import org.apache.spark.sql.types._

val dirPrefix = "/nuclio5"

val eventhubParameters = Map[String, String](
  "eventhubs.name" -> "enrichedfleet",  
  "eventhubs.consumergroup" -> "$Default",
  "eventhubs.progressTrackingDir" -> s"$dirPrefix/progress",
  "eventhubs.policyname" -> "all",
  
  "eventhubs.policykey" -> "nV0AN8ZlZPfA71jz1chUvt/bO3Gmd4YZL5E1DzzPDuw=",
  "eventhubs.namespace" -> "iguaziohackfest",
  "eventhubs.partition.count" -> "2",
  "eventhubs.maxRate" -> s"400" 
)

val checkpointLocation = s"$dirPrefix/checkpoint"

dbutils.fs.rm(s"dbfs:$checkpointLocation", true)

// COMMAND ----------

val inputStream = spark.readStream
  .format("eventhubs")
  .options(eventhubParameters)
  .load()

// COMMAND ----------

val storageAccountName = "tk8abtblobs"
val storageAccountKey = "8essxN36VO+4zIYQEtDJ0+gd6KoHdG9mIvrAcKiKAALAIx2a3u6oqTO8ba8alwboSMnFEOh5wlkxKc8y12pQqQ=="
val containerName = "out5"

val timestamp: Long = System.currentTimeMillis / 1000
val folderName = s"$timestamp"

spark.conf.set(
  s"fs.azure.account.key.$storageAccountName.blob.core.windows.net",
  storageAccountKey)

val containerOutputLocation = s"wasbs://$containerName@$storageAccountName.blob.core.windows.net/$folderName"

// COMMAND ----------

val schema = (new StructType)    
      .add("id", StringType)
      .add("latitude", StringType)
      .add("longitude", StringType)
      .add("tirePressure", FloatType)
      .add("fuelEfficiencyPercentage", FloatType)
      .add("weatherCondition", StringType)

val df1 = inputStream.select($"body".cast("string").as("value")
                             , from_unixtime($"enqueuedTime").cast(TimestampType).as("enqueuedTime")).withWatermark("enqueuedTime", "1 minutes")

val df2 = df1.select(from_json(($"value"), schema).as("body")
                     , $"enqueuedTime")

val df3 = df2.select(
  $"enqueuedTime"
  , $"body.id".cast("integer")
  , $"body.latitude".cast("float")
  , $"body.longitude".cast("float")
  , $"body.tirePressure"
  , $"body.fuelEfficiencyPercentage"
  , $"body.weatherCondition"
)

// COMMAND ----------

val avgfuel = df3
  .groupBy(window($"enqueuedTime", "10 seconds"), $"weatherCondition" )    
  .agg(avg($"fuelEfficiencyPercentage") as "fuel_avg", stddev($"fuelEfficiencyPercentage") as "fuel_stddev")
  .select($"weatherCondition", $"fuel_avg")
  .coalesce(8)

val broadcasted = sc.broadcast(avgfuel)

// COMMAND ----------

val joined = df3.join(broadcasted.value, Seq("weatherCondition"))
                .filter($"fuelEfficiencyPercentage" > $"fuel_avg")
                
val finalDf = joined.coalesce(8)

// COMMAND ----------

val streamingQuery1 = finalDf.writeStream.
      outputMode("append").
      trigger(Trigger.ProcessingTime("10 seconds")).
      option("checkpointLocation", checkpointLocation).
      format("json").option("path", containerOutputLocation).start()
